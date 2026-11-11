"""Private admin service: HTML panel plus a scoped release API.

Human operators authenticate with Argon2 + TOTP and get an HttpOnly session
cookie with CSRF protection. CI authenticates with a separate service token
that is accepted only for release publishing.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import secrets
import time
from collections import defaultdict, deque
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
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
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session as DbSession

from .crypto import (
    effective_expires_at,
    new_key,
    product_signing_env,
    product_signing_key,
    public_key_of,
    sha256_hex,
    sign_manifest,
)
from .db import get_db, init_db
from .models import (
    Activation,
    AuditEvent,
    License,
    PlanPreset,
    Product,
    ProductFeature,
    ProductKey,
    ProductLimit,
    Release,
)

TEMPLATES = Jinja2Templates(directory=str(Path(__file__).resolve().parent.parent / "templates"))

@asynccontextmanager
async def lifespan(_: FastAPI):
    init_db()
    yield


app = FastAPI(title="License desk", version="1.0", lifespan=lifespan)
app.mount(
    "/admin/static",
    StaticFiles(directory=str(Path(__file__).resolve().parent.parent / "static")),
    name="admin-static",
)


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
_CAPTCHAS: dict[str, tuple[str, float, str]] = {}
CAPTCHA_TTL = 300


def _new_captcha() -> tuple[str, str]:
    """Return a short-lived, single-use human-check question and token."""
    now = time.time()
    for token, (_, expires, _) in list(_CAPTCHAS.items()):
        if expires < now:
            _CAPTCHAS.pop(token, None)
    left = secrets.randbelow(8) + 2
    right = secrets.randbelow(8) + 2
    question = f"{left} + {right}"
    token = secrets.token_urlsafe(18)
    _CAPTCHAS[token] = (str(left + right), now + CAPTCHA_TTL, question)
    return token, question


def _check_captcha(token: str, answer: str) -> bool:
    record = _CAPTCHAS.pop(token, None)
    if not record:
        return False
    expected, expires, _ = record
    return expires >= time.time() and secrets.compare_digest(answer.strip(), expected)


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
    token, question = _new_captcha()
    return TEMPLATES.TemplateResponse(
        request,
        "login.html",
        {"error": "", "captcha_token": token, "captcha_question": question},
    )


@app.get("/admin/login/captcha", response_class=HTMLResponse)
def login_captcha(request: Request):
    token, question = _new_captcha()
    return TEMPLATES.TemplateResponse(
        request,
        "_captcha.html",
        {"captcha_token": token, "captcha_question": question},
    )


@app.post("/admin/login")
async def login_submit(request: Request):
    form = dict(await request.form())
    ip = _client_ip(request)
    token, question = _new_captcha()
    context = {
        "error": "",
        "captcha_token": token,
        "captcha_question": question,
    }
    if _login_blocked(ip):
        return TEMPLATES.TemplateResponse(
            request, "login.html", {**context, "error": "Too many attempts. Wait and try again."}, status_code=429
        )
    if not _check_captcha(form.get("captcha_token", ""), form.get("captcha_answer", "")):
        _record_failure(ip)
        return TEMPLATES.TemplateResponse(
            request, "login.html", {**context, "error": "Human check failed. Try the new question."}, status_code=401
        )
    if not _verify_credentials(form.get("password", ""), form.get("totp", "")):
        _record_failure(ip)
        return TEMPLATES.TemplateResponse(
            request, "login.html", {**context, "error": "Invalid credentials."}, status_code=401
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


def _stats_context(db: DbSession) -> dict:
    now = datetime.now(UTC).replace(tzinfo=None)
    active_rows = (
        db.query(License)
        .filter(License.status == "active")
        .order_by(License.id.desc())
        .all()
    )
    expiring = []
    for row in active_rows:
        expires = effective_expires_at(row)
        if expires is not None and expires <= now + timedelta(days=30):
            expiring.append((row, expires))
    expiring.sort(key=lambda item: item[1])
    return {
        "now": now,
        "expiring": expiring,
        "stats": {
            "products": db.query(Product).count(),
            "licenses": db.query(License).count(),
            "active": len(active_rows),
            "activations": db.query(Activation)
            .filter(Activation.revoked_at.is_(None))
            .count(),
            "expiring": len(expiring),
            "expiring_7": sum(
                1 for _, expires in expiring if expires <= now + timedelta(days=7)
            ),
            "lifetime": sum(1 for row in active_rows if row.key_type == "lifetime"),
        },
    }


@app.get("/admin/partials/stats", response_class=HTMLResponse)
def stats_partial(request: Request, db: DbSession = Depends(get_db)):
    _require_session(request)
    context = _stats_context(db)
    return TEMPLATES.TemplateResponse(request, "_stats.html", context)


@app.get("/admin", response_class=HTMLResponse)
def dashboard(request: Request, db: DbSession = Depends(get_db)):
    context = _stats_context(db)
    now = context["now"]
    expiring = context["expiring"]
    stats = context["stats"]
    products = []
    for product in db.query(Product).order_by(Product.slug).all():
        products.append(
            {
                "row": product,
                "keys": db.query(License)
                .filter(License.product_id == product.id)
                .count(),
                "active": db.query(License)
                .filter(
                    License.product_id == product.id,
                    License.status == "active",
                )
                .count(),
                "activations": db.query(Activation)
                .join(License, Activation.license_id == License.id)
                .filter(
                    License.product_id == product.id,
                    Activation.revoked_at.is_(None),
                )
                .count(),
            }
        )
    recent = db.query(AuditEvent).order_by(AuditEvent.id.desc()).limit(8).all()
    product_map = {product.id: product for product in db.query(Product).all()}
    return _render(
        request,
        "dashboard.html",
        stats=stats,
        expiring=expiring[:8],
        products=products,
        recent=recent,
        product_map=product_map,
        now=now,
    )


@app.get("/admin/products", response_class=HTMLResponse)
def products_page(request: Request, db: DbSession = Depends(get_db)):
    products = []
    for product in db.query(Product).order_by(Product.slug).all():
        products.append(
            {
                "row": product,
                "keys": db.query(License).filter(License.product_id == product.id).count(),
                "active": db.query(License)
                .filter(
                    License.product_id == product.id,
                    License.status == "active",
                )
                .count(),
                "presets": db.query(PlanPreset)
                .filter(PlanPreset.product_id == product.id)
                .count(),
            }
        )
    return _render(request, "products.html", products=products)


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


@app.get("/admin/products/{product_id}", response_class=HTMLResponse)
def product_detail(
    request: Request, product_id: int, db: DbSession = Depends(get_db)
):
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    return _render(
        request,
        "product_detail.html",
        product=product,
        signing=db.query(ProductKey)
        .filter(ProductKey.product_id == product.id)
        .first(),
        limits=db.query(ProductLimit)
        .filter(ProductLimit.product_id == product.id)
        .order_by(ProductLimit.limit_key)
        .all(),
        features=db.query(ProductFeature)
        .filter(ProductFeature.product_id == product.id)
        .order_by(ProductFeature.feature_key)
        .all(),
        presets=db.query(PlanPreset)
        .filter(PlanPreset.product_id == product.id)
        .order_by(PlanPreset.name)
        .all(),
        keys=db.query(License)
        .filter(License.product_id == product.id)
        .order_by(License.id.desc())
        .limit(12)
        .all(),
    )


@app.post("/admin/products/{product_id}/status")
async def product_status(
    request: Request, product_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    product.status = "disabled" if product.status == "active" else "active"
    db.commit()
    _audit(db, f"product.{product.status}", product.slug, product.id)
    return RedirectResponse(f"/admin/products/{product.id}", status_code=303)


@app.post("/admin/products/{product_id}/limits")
async def product_limit_create(
    request: Request, product_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    key = form.get("limit_key", "").strip().lower().replace(" ", "_")
    label = form.get("label", "").strip() or key
    value_type = form.get("value_type", "integer")
    if not key:
        raise HTTPException(status_code=400, detail="limit key is required")
    if value_type not in {"integer", "boolean", "string"}:
        raise HTTPException(status_code=400, detail="invalid limit type")
    existing = (
        db.query(ProductLimit)
        .filter(ProductLimit.product_id == product.id, ProductLimit.limit_key == key)
        .first()
    )
    if existing:
        existing.label = label[:80]
        existing.value_type = value_type
        existing.description = form.get("description", "")[:500]
    else:
        db.add(
            ProductLimit(
                product_id=product.id,
                limit_key=key[:40],
                label=label[:80],
                value_type=value_type,
                description=form.get("description", "")[:500],
            )
        )
    db.commit()
    _audit(db, "product.limit", key, product.id)
    return RedirectResponse(f"/admin/products/{product.id}#limits", status_code=303)


@app.post("/admin/products/{product_id}/features")
async def product_feature_create(
    request: Request, product_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    key = form.get("feature_key", "").strip().lower().replace(" ", "_")
    label = form.get("label", "").strip() or key
    if not key:
        raise HTTPException(status_code=400, detail="feature key is required")
    existing = (
        db.query(ProductFeature)
        .filter(
            ProductFeature.product_id == product.id,
            ProductFeature.feature_key == key,
        )
        .first()
    )
    if existing:
        existing.label = label[:80]
        existing.description = form.get("description", "")[:500]
    else:
        db.add(
            ProductFeature(
                product_id=product.id,
                feature_key=key[:40],
                label=label[:80],
                description=form.get("description", "")[:500],
            )
        )
    db.commit()
    _audit(db, "product.feature", key, product.id)
    return RedirectResponse(f"/admin/products/{product.id}#features", status_code=303)


@app.post("/admin/products/{product_id}/presets")
async def product_preset_create(
    request: Request, product_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    name = form.get("name", "").strip()
    if not name:
        raise HTTPException(status_code=400, detail="preset name is required")
    key_type = form.get("key_type", "timed")
    if key_type not in {"trial", "timed", "lifetime"}:
        raise HTTPException(status_code=400, detail="invalid key type")
    try:
        limits = json.loads(form.get("limits") or "{}")
        features = json.loads(form.get("features") or "[]")
        days = int(form.get("validity_days") or 30)
        trial_minutes = int(form.get("trial_minutes") or 15)
        max_activations = int(form.get("max_activations") or 1)
        grace_seconds = int(form.get("grace_seconds") or 0)
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="invalid preset values")
    if not isinstance(limits, dict) or not isinstance(features, list):
        raise HTTPException(status_code=400, detail="invalid preset JSON")
    if key_type == "lifetime":
        days = None
        trial_minutes = None
        starts = False
        one_time = False
    elif key_type == "trial":
        days = None
        starts = True
        one_time = True
        max_activations = 1
        grace_seconds = 0
    else:
        starts = form.get("starts_on_activation") in {"1", "on", "true", "yes"}
        one_time = form.get("one_time_activation") in {"1", "on", "true", "yes"}
    db.add(
        PlanPreset(
            product_id=product.id,
            name=name[:40],
            key_type=key_type,
            trial_minutes=trial_minutes,
            validity_days=days,
            starts_on_activation=starts,
            one_time_activation=one_time,
            grace_seconds=grace_seconds,
            max_activations=max_activations,
            limits_json=json.dumps(limits, sort_keys=True),
            features_json=json.dumps(features),
            notes=form.get("notes", "")[:500],
        )
    )
    db.commit()
    _audit(db, "product.preset", name, product.id)
    return RedirectResponse(f"/admin/products/{product.id}#plans", status_code=303)


@app.post("/admin/products/{product_id}/limits/{limit_id}/delete")
async def product_limit_delete(
    request: Request, product_id: int, limit_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    row = (
        db.query(ProductLimit)
        .filter(
            ProductLimit.id == limit_id,
            ProductLimit.product_id == product_id,
        )
        .first()
    )
    if row:
        db.delete(row)
        db.commit()
        _audit(db, "product.limit.delete", row.limit_key, product_id)
    return RedirectResponse(f"/admin/products/{product_id}#limits", status_code=303)


@app.post("/admin/products/{product_id}/features/{feature_id}/delete")
async def product_feature_delete(
    request: Request,
    product_id: int,
    feature_id: int,
    db: DbSession = Depends(get_db),
):
    form = dict(await request.form())
    _check_csrf(request, form)
    row = (
        db.query(ProductFeature)
        .filter(
            ProductFeature.id == feature_id,
            ProductFeature.product_id == product_id,
        )
        .first()
    )
    if row:
        db.delete(row)
        db.commit()
        _audit(db, "product.feature.delete", row.feature_key, product_id)
    return RedirectResponse(f"/admin/products/{product_id}#features", status_code=303)


@app.post("/admin/products/{product_id}/presets/{preset_id}/delete")
async def product_preset_delete(
    request: Request,
    product_id: int,
    preset_id: int,
    db: DbSession = Depends(get_db),
):
    form = dict(await request.form())
    _check_csrf(request, form)
    row = (
        db.query(PlanPreset)
        .filter(PlanPreset.id == preset_id, PlanPreset.product_id == product_id)
        .first()
    )
    if row:
        db.delete(row)
        db.commit()
        _audit(db, "product.preset.delete", row.name, product_id)
    return RedirectResponse(f"/admin/products/{product_id}#plans", status_code=303)


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
        product_map={p.id: p for p in db.query(Product).all()},
        presets=db.query(PlanPreset).order_by(PlanPreset.name).all(),
        preset_data=[
            {
                "id": preset.id,
                "product_id": preset.product_id,
                "name": preset.name,
                "key_type": preset.key_type,
                "trial_minutes": preset.trial_minutes,
                "validity_days": preset.validity_days,
                "starts_on_activation": preset.starts_on_activation,
                "one_time_activation": preset.one_time_activation,
                "grace_seconds": preset.grace_seconds,
                "max_activations": preset.max_activations,
                "limits_json": preset.limits_json,
                "features_json": preset.features_json,
            }
            for preset in db.query(PlanPreset).order_by(PlanPreset.name).all()
        ],
        selected_product=product_id,
    )


@app.post("/admin/products/{product_id}/licenses")
async def license_create(request: Request, product_id: int, db: DbSession = Depends(get_db)):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    preset = None
    if form.get("preset_id"):
        try:
            preset_id = int(form["preset_id"])
        except ValueError:
            raise HTTPException(status_code=400, detail="invalid preset")
        preset = (
            db.query(PlanPreset)
            .filter(
                PlanPreset.id == preset_id,
                PlanPreset.product_id == product.id,
            )
            .first()
        )
        if not preset:
            raise HTTPException(status_code=404, detail="Unknown preset")
        form["key_type"] = preset.key_type
        form["trial_minutes"] = str(preset.trial_minutes or 15)
        form["days"] = str(preset.validity_days or 365)
        form["starts_on_activation"] = "1" if preset.starts_on_activation else ""
        form["one_time_activation"] = "1" if preset.one_time_activation else ""
        form["grace_seconds"] = str(preset.grace_seconds)
        form["max_activations"] = str(preset.max_activations)
        form["limits"] = preset.limits_json
        form["features"] = preset.features_json
        form["plan_name"] = preset.name
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

    key_type = str(form.get("key_type") or "timed").strip().lower()
    if key_type not in {"trial", "timed", "lifetime"}:
        raise HTTPException(
            status_code=400, detail="key_type must be trial, timed or lifetime"
        )
    try:
        days = int(form.get("days") or 365)
        trial_minutes = int(form.get("trial_minutes") or 15)
        max_activations = int(form.get("max_activations") or 1)
        grace_seconds = int(form.get("grace_seconds") or 86400)
    except (TypeError, ValueError):
        raise HTTPException(
            status_code=400,
            detail="days, trial_minutes, max_activations and grace_seconds must be integers",
        )
    if key_type == "timed" and not 1 <= days <= 3650:
        raise HTTPException(status_code=400, detail="days must be between 1 and 3650")
    if key_type == "trial" and not 1 <= trial_minutes <= 525600:
        raise HTTPException(
            status_code=400, detail="trial_minutes must be between 1 and 525600"
        )
    if not 1 <= max_activations <= 1000:
        raise HTTPException(
            status_code=400,
            detail="max_activations must be between 1 and 1000",
        )
    if not 0 <= grace_seconds <= 31536000:
        raise HTTPException(
            status_code=400, detail="grace_seconds must be between 0 and 31536000"
        )
    if key_type == "trial":
        starts_on_activation = True
        one_time_activation = True
        grace_seconds = 0
        max_activations = 1
        validity_days = None
        expiry = datetime.now(UTC).replace(tzinfo=None) + timedelta(days=3650)
    elif key_type == "lifetime":
        starts_on_activation = False
        one_time_activation = False
        validity_days = None
        expiry = datetime(9999, 12, 31)
    else:
        starts_on_activation = form.get("starts_on_activation") in {
            "1",
            "true",
            "on",
            "yes",
        }
        one_time_activation = form.get("one_time_activation") in {
            "1",
            "true",
            "on",
            "yes",
        }
        validity_days = days
        expiry = datetime.now(UTC).replace(tzinfo=None) + timedelta(days=days)

    product_key = (
        db.query(ProductKey)
        .filter(ProductKey.product_id == product.id)
        .first()
    )
    key = new_key((product_key.key_prefix if product_key else None) or "reo")
    plan_name = (form.get("plan_name") or "standard").strip()[:40] or "standard"
    row = License(
        product_id=product.id,
        plan_preset_id=preset.id if preset else None,
        key_hash=sha256_hex(key),
        key_last4=key[-4:],
        plan_name=plan_name,
        key_type=key_type,
        trial_minutes=trial_minutes if key_type == "trial" else None,
        validity_days=validity_days,
        starts_on_activation=starts_on_activation,
        one_time_activation=one_time_activation,
        grace_seconds=grace_seconds,
        limits_json=json.dumps(limits, sort_keys=True),
        features_json=json.dumps(features),
        expires_at=expiry,
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


@app.get("/admin/licenses/{license_id}/edit", response_class=HTMLResponse)
def license_edit(
    request: Request, license_id: int, db: DbSession = Depends(get_db)
):
    row = db.query(License).filter(License.id == license_id).first()
    if not row:
        raise HTTPException(status_code=404, detail="Unknown license")
    return _render(
        request,
        "license_edit.html",
        license=row,
        product=db.query(Product).filter(Product.id == row.product_id).first(),
        presets=db.query(PlanPreset)
        .filter(PlanPreset.product_id == row.product_id)
        .order_by(PlanPreset.name)
        .all(),
    )


@app.post("/admin/licenses/{license_id}/edit")
async def license_edit_save(
    request: Request, license_id: int, db: DbSession = Depends(get_db)
):
    form = dict(await request.form())
    _check_csrf(request, form)
    row = db.query(License).filter(License.id == license_id).first()
    if not row:
        raise HTTPException(status_code=404, detail="Unknown license")
    try:
        limits = json.loads(form.get("limits") or "{}")
        features = json.loads(form.get("features") or "[]")
        max_activations = int(form.get("max_activations") or row.max_activations)
        grace_seconds = int(form.get("grace_seconds") or row.grace_seconds)
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="invalid JSON or numbers")
    if not isinstance(limits, dict) or not isinstance(features, list):
        raise HTTPException(status_code=400, detail="limits/features have the wrong shape")
    if not 1 <= max_activations <= 1000:
        raise HTTPException(status_code=400, detail="max_activations is out of range")
    row.plan_name = (form.get("plan_name") or row.plan_name).strip()[:40]
    row.notes = form.get("notes", row.notes)[:1000]
    row.limits_json = json.dumps(limits, sort_keys=True)
    row.features_json = json.dumps(features)
    row.max_activations = max_activations
    row.grace_seconds = max(0, grace_seconds)
    preset_id = form.get("preset_id")
    if preset_id:
        try:
            preset = (
                db.query(PlanPreset)
                .filter(
                    PlanPreset.id == int(preset_id),
                    PlanPreset.product_id == row.product_id,
                )
                .first()
            )
        except ValueError:
            preset = None
        row.plan_preset_id = preset.id if preset else None
    if row.key_type != "lifetime":
        raw = form.get("expires_at", "").strip()
        if raw:
            try:
                row.expires_at = datetime.fromisoformat(raw)
            except ValueError:
                raise HTTPException(status_code=400, detail="invalid expiry")
        if row.key_type == "trial":
            try:
                row.trial_minutes = int(form.get("trial_minutes") or row.trial_minutes or 15)
            except (TypeError, ValueError):
                raise HTTPException(status_code=400, detail="invalid trial minutes")
        elif row.key_type == "timed":
            try:
                row.validity_days = int(form.get("validity_days") or row.validity_days or 30)
            except (TypeError, ValueError):
                raise HTTPException(status_code=400, detail="invalid validity days")
    db.commit()
    _audit(db, "license.edit", str(row.id), row.product_id)
    return RedirectResponse(f"/admin/licenses/{row.id}", status_code=303)


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
        if row.key_type == "lifetime":
            raise HTTPException(status_code=400, detail="Lifetime keys do not expire")
        try:
            days = int(form.get("days") or 30)
        except (TypeError, ValueError):
            raise HTTPException(status_code=400, detail="days must be an integer")
        if not 1 <= days <= 3650:
            raise HTTPException(status_code=400, detail="days must be between 1 and 3650")
        if row.activation_expires_at is not None:
            row.activation_expires_at = row.activation_expires_at + timedelta(days=days)
        else:
            row.expires_at = row.expires_at + timedelta(days=days)
            if row.starts_on_activation and row.key_type == "timed":
                row.validity_days = (row.validity_days or 0) + days
            elif row.key_type == "trial":
                row.trial_minutes = (row.trial_minutes or 0) + days * 1440
    elif action == "reset":
        now = datetime.now(UTC).replace(tzinfo=None)
        for activation in row.activations:
            activation.revoked_at = now
        row.first_activated_at = None
        row.activation_expires_at = None
        row.used_at = None
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
        product_map={p.id: p for p in db.query(Product).all()},
    )


@app.post("/admin/releases/upload")
async def release_upload(
    request: Request,
    product_id: int = Form(...),
    version: str = Form(...),
    channel: str = Form("stable"),
    platform: str = Form("windows"),
    min_version: str = Form("0.0.0"),
    mandatory: bool = Form(False),
    security: bool = Form(False),
    notes: str = Form(""),
    artifact: UploadFile = File(...),
    db: DbSession = Depends(get_db),
):
    form = dict(await request.form())
    _check_csrf(request, form)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    if not re.fullmatch(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        raise HTTPException(status_code=400, detail="version must look like 1.2.3")
    if channel not in {"stable", "beta"}:
        raise HTTPException(status_code=400, detail="channel must be stable or beta")
    if platform not in {"windows", "linux", "macos"}:
        raise HTTPException(status_code=400, detail="unknown platform")
    target = _artifact_target(product.slug, version)
    digest = hashlib.sha256()
    total = 0
    with target.open("wb") as handle:
        while chunk := await artifact.read(1024 * 256):
            total += len(chunk)
            if total > 600 * 1024 * 1024:
                target.unlink(missing_ok=True)
                raise HTTPException(status_code=413, detail="artifact is too large")
            digest.update(chunk)
            handle.write(chunk)
    row = _publish_release(
        db,
        product,
        version=version,
        channel=channel,
        platform=platform,
        min_version=min_version,
        mandatory=mandatory,
        security=security,
        notes=notes[:2000],
        artifact_path=str(target),
        sha256=digest.hexdigest(),
        actor="operator",
    )
    return RedirectResponse(f"/admin/releases#release-{row.id}", status_code=303)


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
