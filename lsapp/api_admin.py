"""Private admin service: HTML panel plus a scoped release API.

Human operators authenticate with Argon2 + TOTP and get an HttpOnly session
cookie with CSRF protection. CI authenticates with a separate service token
that is accepted only for release publishing.
"""

from __future__ import annotations

from contextlib import asynccontextmanager
import hashlib
import json
import os
import secrets
from collections import defaultdict, deque
from datetime import datetime, timedelta, UTC
from pathlib import Path

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError
from fastapi import (
    Depends,
    FastAPI,
    File,
    Form,
    Header,
    HTTPException,
    Request,
    Response,
    UploadFile,
)
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.templating import Jinja2Templates
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session as DbSession

from .crypto import (
    new_key,
    product_signing_env,
    product_signing_key,
    public_key_of,
    sha256_hex,
    sign_manifest,
)
from .db import get_db, init_db
from .models import Activation, AuditEvent, License, Product, ProductKey, Release

TEMPLATES = Jinja2Templates(directory=str(Path(__file__).resolve().parent.parent / "templates"))

@asynccontextmanager
async def lifespan(_: FastAPI):
    init_db()
    yield


app = FastAPI(title="ReOrchestra license admin", version="1.0", lifespan=lifespan)


_hasher = PasswordHasher()

SESSION_COOKIE = "reo_admin"
SESSION_HOURS = int(os.getenv("ADMIN_SESSION_HOURS", "8"))
SECURE_COOKIE = os.getenv("ADMIN_COOKIE_SECURE", "1") == "1"
MAX_LOGIN_FAILURES = int(os.getenv("ADMIN_LOGIN_MAX_FAILURES", "5"))
LOGIN_LOCK_MINUTES = int(os.getenv("ADMIN_LOGIN_LOCK_MINUTES", "15"))
ALLOWED_SERVICE_SCOPE = "release.publish"


class AdminSession:
    def __init__(self, actor: str):
        self.actor = actor
        self.csrf = secrets.token_urlsafe(24)
        self.expires_at = datetime.now(UTC) + timedelta(hours=SESSION_HOURS)

    def expired(self) -> bool:
        return datetime.now(UTC) >= self.expires_at


SESSIONS: dict[str, AdminSession] = {}
_FAILURES: dict[str, deque] = defaultdict(deque)


def _client_ip(request: Request) -> str:
    return (request.client.host if request.client else "") or "unknown"


def _login_blocked(ip: str) -> bool:
    hits = _FAILURES[ip]
    cutoff = datetime.now(UTC) - timedelta(minutes=LOGIN_LOCK_MINUTES)
    while hits and hits[0] < cutoff:
        hits.popleft()
    return len(hits) >= MAX_LOGIN_FAILURES


def _record_failure(ip: str) -> None:
    _FAILURES[ip].append(datetime.now(UTC))


def _session(request: Request) -> AdminSession | None:
    token = request.cookies.get(SESSION_COOKIE, "")
    session = SESSIONS.get(token)
    if session and session.expired():
        SESSIONS.pop(token, None)
        return None
    return session


class LoginRequired(Exception):
    """Raised by panel routes so the handler can redirect instead of erroring."""


@app.exception_handler(LoginRequired)
def _redirect_to_login(request: Request, exc: LoginRequired):
    return RedirectResponse("/admin/login", status_code=303)


def _require_session(request: Request) -> AdminSession:
    session = _session(request)
    if not session:
        raise LoginRequired()
    return session


def _check_csrf(request: Request, form: dict) -> None:
    session = _session(request)
    if not session or form.get("csrf_token") != session.csrf:
        raise HTTPException(status_code=403, detail="Invalid CSRF token")


def _require_service_token(token: str | None, scope: str) -> None:
    expected = os.getenv("ADMIN_SERVICE_TOKEN", "")
    if not expected or not token or not secrets.compare_digest(token, expected):
        raise HTTPException(status_code=401, detail="Service token required")
    if scope != ALLOWED_SERVICE_SCOPE:
        raise HTTPException(status_code=403, detail="Token scope does not allow this action")


def _audit(db: DbSession, action: str, target: str = "", product_id: int | None = None, actor: str = "operator", **meta):
    db.add(
        AuditEvent(
            action=action,
            target=target,
            product_id=product_id,
            actor=actor,
            metadata_json=json.dumps(meta),
        )
    )
    db.commit()


def _verify_credentials(password: str, totp: str) -> bool:
    import pyotp

    expected = os.getenv("LICENSE_ADMIN_PASSWORD_HASH", "")
    if not expected:
        return False
    try:
        _hasher.verify(expected, password)
    except VerifyMismatchError:
        return False
    secret = os.getenv("LICENSE_ADMIN_TOTP_SECRET", "")
    return bool(secret) and pyotp.TOTP(secret).verify(totp, valid_window=1)


def _set_session_cookie(response: Response, token: str) -> None:
    response.set_cookie(
        SESSION_COOKIE,
        token,
        httponly=True,
        samesite="strict",
        secure=SECURE_COOKIE,
        max_age=SESSION_HOURS * 3600,
        path="/",
    )


# ---------------------------------------------------------------- HTML panel


@app.get("/admin/login", response_class=HTMLResponse)
def login_page(request: Request):
    return TEMPLATES.TemplateResponse(request, "login.html", {"error": ""})


@app.post("/admin/login")
async def login_submit(request: Request):
    form = dict(await request.form())
    ip = _client_ip(request)
    if _login_blocked(ip):
        return TEMPLATES.TemplateResponse(
            request, "login.html", {"error": "Too many attempts. Wait and try again."}, status_code=429
        )
    if not _verify_credentials(form.get("password", ""), form.get("totp", "")):
        _record_failure(ip)
        return TEMPLATES.TemplateResponse(
            request, "login.html", {"error": "Invalid credentials."}, status_code=401
        )
    _FAILURES.pop(ip, None)
    token = secrets.token_urlsafe(32)
    SESSIONS[token] = AdminSession(os.getenv("LICENSE_ADMIN_ACTOR", "operator"))
    response = RedirectResponse("/admin", status_code=303)
    _set_session_cookie(response, token)
    return response


@app.post("/admin/logout")
async def logout(request: Request):
    form = dict(await request.form())
    _check_csrf(request, form)
    token = request.cookies.get(SESSION_COOKIE, "")
    SESSIONS.pop(token, None)
    response = RedirectResponse("/admin/login", status_code=303)
    response.delete_cookie(SESSION_COOKIE, path="/")
    return response


def _render(request: Request, name: str, **context):
    session = _require_session(request)
    return TEMPLATES.TemplateResponse(
        request, name, {"session": session, "csrf": session.csrf, **context}
    )


@app.get("/admin", response_class=HTMLResponse)
def dashboard(request: Request, db: DbSession = Depends(get_db)):
    soon = datetime.now(UTC).replace(tzinfo=None) + timedelta(days=30)
    stats = {
        "products": db.query(Product).count(),
        "licenses": db.query(License).count(),
        "active": db.query(License).filter(License.status == "active").count(),
        "activations": db.query(Activation).filter(Activation.revoked_at.is_(None)).count(),
        "expiring": db.query(License)
        .filter(License.status == "active", License.expires_at <= soon)
        .count(),
    }
    recent = db.query(AuditEvent).order_by(AuditEvent.id.desc()).limit(10).all()
    return _render(request, "dashboard.html", stats=stats, recent=recent)


@app.get("/admin/products", response_class=HTMLResponse)
def products_page(request: Request, db: DbSession = Depends(get_db)):
    return _render(request, "products.html", products=db.query(Product).order_by(Product.slug).all())


@app.post("/admin/products")
async def product_create(request: Request, db: DbSession = Depends(get_db)):
    form = dict(await request.form())
    _check_csrf(request, form)
    slug = form.get("slug", "").strip().lower()
    name = form.get("name", "").strip()
    if not slug or not name:
        raise HTTPException(status_code=400, detail="slug and name are required")
    if not db.query(Product).filter(Product.slug == slug).first():
        db.add(Product(slug=slug, name=name))
        db.commit()
        _audit(db, "product.create", slug)
    return RedirectResponse("/admin/products", status_code=303)


@app.post("/admin/products/{product_id}/keys")
async def product_keys(request: Request, product_id: int, db: DbSession = Depends(get_db)):
    """Register the public halves of this product's own signing keys."""
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    license_env = product_signing_env(product.slug, "license")
    update_env = product_signing_env(product.slug, "update")
    row = (
        db.query(ProductKey)
        .filter(ProductKey.product_id == product.id)
        .first()
    )
    if row is None:
        row = ProductKey(product_id=product.id)
        db.add(row)
    row.license_public_key = public_key_of(
        product_signing_key(product.slug, "license")
    )
    row.update_public_key = public_key_of(
        product_signing_key(product.slug, "update")
    )
    row.key_prefix = (form.get("prefix") or "reo")[:12] or "reo"
    row.license_key_ref = license_env
    row.update_key_ref = update_env
    db.commit()
    _audit(db, "product.keys", product.slug, product.id, license=license_env, update=update_env)
    return RedirectResponse("/admin/products", status_code=303)


@app.get("/admin/licenses", response_class=HTMLResponse)
def licenses_page(request: Request, product_id: int | None = None, db: DbSession = Depends(get_db)):
    query = db.query(License)
    if product_id:
        query = query.filter(License.product_id == product_id)
    return _render(
        request,
        "licenses.html",
        licenses=query.order_by(License.id.desc()).all(),
        products=db.query(Product).order_by(Product.slug).all(),
        selected_product=product_id,
    )


@app.post("/admin/products/{product_id}/licenses")
async def license_create(request: Request, product_id: int, db: DbSession = Depends(get_db)):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    try:
        limits = json.loads(form.get("limits") or "{}")
        features = json.loads(form.get("features") or "[]")
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="limits/features must be JSON")
    if not isinstance(limits, dict):
        raise HTTPException(status_code=400, detail="limits must be a JSON object")
    if not isinstance(features, list) or any(
        not isinstance(feature, str) for feature in features
    ):
        raise HTTPException(
            status_code=400, detail="features must be a JSON array of strings"
        )
    max_accounts = limits.get("max_accounts")
    if max_accounts is not None and (
        isinstance(max_accounts, bool)
        or not isinstance(max_accounts, int)
        or max_accounts < 1
    ):
        raise HTTPException(
            status_code=400, detail="limits.max_accounts must be a positive integer"
        )
    try:
        days = int(form.get("days") or 365)
        max_activations = int(form.get("max_activations") or 1)
    except (TypeError, ValueError):
        raise HTTPException(
            status_code=400, detail="days and max_activations must be integers"
        )
    if not 1 <= days <= 3650:
        raise HTTPException(status_code=400, detail="days must be between 1 and 3650")
    if not 1 <= max_activations <= 1000:
        raise HTTPException(
            status_code=400,
            detail="max_activations must be between 1 and 1000",
        )

    product_key = (
        db.query(ProductKey)
        .filter(ProductKey.product_id == product.id)
        .first()
    )
    key = new_key((product_key.key_prefix if product_key else None) or "reo")
    plan_name = (form.get("plan_name") or "standard").strip()[:40] or "standard"
    row = License(
        product_id=product.id,
        key_hash=sha256_hex(key),
        key_last4=key[-4:],
        plan_name=plan_name,
        limits_json=json.dumps(limits, sort_keys=True),
        features_json=json.dumps(features),
        expires_at=datetime.now(UTC).replace(tzinfo=None)
        + timedelta(days=days),
        max_activations=max_activations,
        notes=form.get("notes", ""),
    )
    db.add(row)
    db.commit()
    _audit(
        db,
        "license.issue",
        str(row.id),
        product.id,
        key_last4=key[-4:],
        plan=row.plan_name,
    )
    return _render(
        request,
        "license_created.html",
        product=product,
        license=row,
        raw_key=key,
    )


@app.get("/admin/licenses/{license_id}", response_class=HTMLResponse)
def license_detail(request: Request, license_id: int, db: DbSession = Depends(get_db)):
    row = db.query(License).filter(License.id == license_id).first()
    if not row:
        raise HTTPException(status_code=404, detail="Unknown license")
    product = db.query(Product).filter(Product.id == row.product_id).first()
    events = (
        db.query(AuditEvent)
        .filter(AuditEvent.target == str(license_id))
        .order_by(AuditEvent.id.desc())
        .limit(20)
        .all()
    )
    return _render(
        request, "license_detail.html", license=row, product=product, events=events
    )


@app.post("/admin/licenses/{license_id}/{action}")
async def license_action(request: Request, license_id: int, action: str, db: DbSession = Depends(get_db)):
    form = dict(await request.form())
    _check_csrf(request, form)
    row = db.query(License).filter(License.id == license_id).first()
    if not row:
        raise HTTPException(status_code=404, detail="Unknown license")
    if action == "revoke":
        row.status = "revoked"
        for activation in row.activations:
            activation.revoked_at = datetime.now(UTC).replace(tzinfo=None)
    elif action == "extend":
        row.expires_at = row.expires_at + timedelta(days=int(form.get("days") or 30))
    elif action == "reset":
        for activation in row.activations:
            activation.revoked_at = datetime.now(UTC).replace(tzinfo=None)
    else:
        raise HTTPException(status_code=400, detail="Unknown action")
    db.commit()
    _audit(db, f"license.{action}", str(license_id), row.product_id)
    return RedirectResponse(f"/admin/licenses/{license_id}", status_code=303)


@app.get("/admin/releases", response_class=HTMLResponse)
def releases_page(request: Request, db: DbSession = Depends(get_db)):
    return _render(
        request,
        "releases.html",
        releases=db.query(Release).order_by(Release.id.desc()).limit(50).all(),
        products=db.query(Product).order_by(Product.slug).all(),
    )


@app.get("/admin/audit", response_class=HTMLResponse)
def audit_page(request: Request, db: DbSession = Depends(get_db)):
    return _render(
        request,
        "audit.html",
        events=db.query(AuditEvent).order_by(AuditEvent.id.desc()).limit(200).all(),
    )


# ------------------------------------------------------------ release upload


class ReleaseRequest(BaseModel):
    version: str = Field(min_length=5, max_length=24)
    channel: str = "stable"
    platform: str = "windows"
    sha256: str = Field(min_length=64, max_length=64)
    artifact_path: str
    mandatory: bool = False
    security: bool = False
    notes: str = ""


def artifacts_dir() -> Path:
    """Read per call so both containers can share a mounted volume."""
    return Path(os.getenv("LICENSE_ARTIFACTS_DIR", "artifacts"))
PUBLIC_BASE_URL = os.getenv("PUBLIC_BASE_URL", "").rstrip("/")


def _artifact_target(slug: str, version: str) -> Path:
    target = artifacts_dir() / slug / version / "app.zip"
    target.parent.mkdir(parents=True, exist_ok=True)
    return target


def _manifest_for(product: Product, row: Release, base_url: str = "") -> dict:
    return {
        "product": product.slug,
        "version": row.version,
        "channel": row.channel,
        "platform": row.platform,
        "min_version": row.min_version,
        "mandatory": row.mandatory,
        "security": row.security,
        "sha256": row.sha256,
        "notes": row.notes,
    }


def _publish_release(
    db: DbSession,
    product: Product,
    *,
    version: str,
    channel: str,
    platform: str = "windows",
    min_version: str = "0.0.0",
    mandatory: bool = False,
    security: bool = False,
    notes: str = "",
    artifact_path: str,
    sha256: str,
    actor: str,
) -> Release:
    """Create or replace one release row, then sign its canonical manifest."""
    row = (
        db.query(Release)
        .filter(
            Release.product_id == product.id,
            Release.version == version,
            Release.channel == channel,
        )
        .first()
    )
    if row is None:
        row = Release(product_id=product.id, version=version, channel=channel)
        db.add(row)
    row.platform = platform
    row.min_version = min_version
    row.mandatory = mandatory
    row.security = security
    row.notes = notes
    row.artifact_path = artifact_path
    row.sha256 = sha256
    row.status = "published"
    db.commit()

    manifest = _manifest_for(product, row, PUBLIC_BASE_URL)
    row.manifest_json = json.dumps(manifest, sort_keys=True)
    row.manifest_signature = sign_manifest(
        manifest, product_signing_key(product.slug, "update")
    )
    db.commit()
    _audit(db, "release.publish", f"{product.slug} {version}", product.id, actor=actor)
    return row


@app.post("/v1/admin/products/{slug}/releases")
async def publish_release(
    slug: str,
    version: str = Form(...),
    channel: str = Form("stable"),
    platform: str = Form("windows"),
    min_version: str = Form("0.0.0"),
    mandatory: bool = Form(False),
    security: bool = Form(False),
    notes: str = Form(""),
    sha256: str = Form(""),
    artifact: UploadFile = File(...),
    x_service_token: str | None = Header(default=None),
    db: DbSession = Depends(get_db),
):
    """CI-only path: multipart upload, service token, no human session."""
    _require_service_token(x_service_token, ALLOWED_SERVICE_SCOPE)
    product = db.query(Product).filter(Product.slug == slug).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")

    target = _artifact_target(slug, version)
    digest = hashlib.sha256()
    with target.open("wb") as handle:
        while chunk := await artifact.read(1024 * 256):
            digest.update(chunk)
            handle.write(chunk)
    actual = digest.hexdigest()
    if sha256 and not secrets.compare_digest(sha256.lower(), actual):
        target.unlink(missing_ok=True)
        raise HTTPException(status_code=400, detail="Checksum mismatch")

    row = _publish_release(
        db,
        product,
        version=version,
        channel=channel,
        platform=platform,
        min_version=min_version,
        mandatory=mandatory,
        security=security,
        notes=notes,
        artifact_path=str(target),
        sha256=actual,
        actor="ci",
    )
    return {
        "id": row.id,
        "version": row.version,
        "channel": row.channel,
        "sha256": row.sha256,
        "status": row.status,
    }
