"""Private admin API. Deploy behind VPN/allowlist; never expose publicly."""

from __future__ import annotations

import json
import os
from datetime import datetime, timedelta, UTC

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError
from fastapi import Depends, FastAPI, Header, HTTPException, Request
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from .crypto import key_from_env, new_key, public_key_of, sha256_hex
from .db import get_db
from .models import AuditEvent, License, Product, ProductKey, Release

app = FastAPI(title="ReOrchestra license admin", version="1.0")
_hasher = PasswordHasher()
SESSIONS: dict[str, datetime] = {}


def _require_session(token: str | None) -> str:
    actor = SESSIONS.get(token or "")
    if not actor or actor < datetime.now(UTC):
        raise HTTPException(status_code=401, detail="Admin session required")
    return token  # type: ignore[return-value]


def _audit(db: Session, action: str, target: str = "", product_id: int | None = None, **meta):
    db.add(AuditEvent(action=action, target=target, product_id=product_id,
                       metadata_json=json.dumps(meta)))
    db.commit()


class LoginRequest(BaseModel):
    password: str
    totp: str = Field(min_length=6, max_length=8)


class ProductRequest(BaseModel):
    slug: str = Field(min_length=2, max_length=40)
    name: str


class KeyRequest(BaseModel):
    plan_name: str = "standard"
    limits: dict = Field(default_factory=dict)
    features: list[str] = Field(default_factory=list)
    days: int = Field(default=365, ge=1, le=3650)
    max_activations: int = Field(default=1, ge=1, le=1000)
    notes: str = ""


@app.post("/v1/admin/login")
def login(request: LoginRequest, http: Request):
    import pyotp

    expected = os.getenv("LICENSE_ADMIN_PASSWORD_HASH", "")
    if not expected:
        raise HTTPException(status_code=503, detail="Admin password not configured")
    try:
        _hasher.verify(expected, request.password)
    except VerifyMismatchError:
        raise HTTPException(status_code=401, detail="Invalid credentials")
    secret = os.getenv("LICENSE_ADMIN_TOTP_SECRET", "")
    if not secret or not pyotp.TOTP(secret).verify(request.totp, valid_window=1):
        raise HTTPException(status_code=401, detail="Invalid credentials")

    import secrets as _secrets

    token = _secrets.token_urlsafe(32)
    SESSIONS[token] = datetime.now(UTC) + timedelta(hours=8)
    return {"token": token, "expires_in": 8 * 3600}


@app.post("/v1/admin/products")
def create_product(
    request: ProductRequest, db: Session = Depends(get_db), x_admin_token: str | None = Header(default=None)
):
    _require_session(x_admin_token)
    product = db.query(Product).filter(Product.slug == request.slug).first()
    if not product:
        product = Product(slug=request.slug, name=request.name)
        db.add(product)
        db.commit()
    _audit(db, "product.create", request.slug, product.id)
    return {"id": product.id, "slug": product.slug}


@app.get("/v1/admin/products")
def list_products(db: Session = Depends(get_db), x_admin_token: str | None = Header(default=None)):
    _require_session(x_admin_token)
    return [{"id": p.id, "slug": p.slug, "name": p.name, "status": p.status} for p in db.query(Product)]


@app.post("/v1/admin/products/{product_id}/keys")
def create_product_keys(
    product_id: int, db: Session = Depends(get_db), x_admin_token: str | None = Header(default=None)
):
    """Register the public halves the client and updater pin."""
    _require_session(x_admin_token)
    license_private = key_from_env("LICENSE_SIGNING_PRIVATE_KEY")
    update_private = key_from_env("UPDATE_SIGNING_PRIVATE_KEY")
    row = ProductKey(
        product_id=product_id,
        license_public_key=public_key_of(license_private),
        update_public_key=public_key_of(update_private),
        key_prefix=os.getenv("LICENSE_KEY_PREFIX", "reo"),
    )
    db.add(row)
    db.commit()
    _audit(db, "product.keys", str(product_id), product_id)
    return {
        "license_public_key": row.license_public_key,
        "update_public_key": row.update_public_key,
        "key_prefix": row.key_prefix,
    }


@app.post("/v1/admin/products/{product_id}/licenses")
def issue_license(
    product_id: int,
    request: KeyRequest,
    db: Session = Depends(get_db),
    x_admin_token: str | None = Header(default=None),
):
    _require_session(x_admin_token)
    product = db.query(Product).filter(Product.id == product_id).first()
    if not product:
        raise HTTPException(status_code=404, detail="Unknown product")
    prefix = os.getenv("LICENSE_KEY_PREFIX", "reo")
    key = new_key(prefix)
    row = License(
        product_id=product_id,
        key_hash=sha256_hex(key),
        key_last4=key[-4:],
        plan_name=request.plan_name,
        limits_json=json.dumps(request.limits),
        features_json=json.dumps(request.features),
        expires_at=datetime.now(UTC).replace(tzinfo=None) + timedelta(days=request.days),
        max_activations=request.max_activations,
        notes=request.notes,
    )
    db.add(row)
    db.commit()
    _audit(db, "license.issue", key[-4:], product_id, plan=request.plan_name)
    # The raw key is shown once and never stored.
    return {"key": key, "id": row.id, "last4": row.key_last4, "expires_at": row.expires_at}


@app.post("/v1/admin/licenses/{license_id}/{action}")
def license_action(
    license_id: int,
    action: str,
    days: int = 30,
    db: Session = Depends(get_db),
    x_admin_token: str | None = Header(default=None),
):
    _require_session(x_admin_token)
    row = db.query(License).filter(License.id == license_id).first()
    if not row:
        raise HTTPException(status_code=404, detail="Unknown license")
    if action == "revoke":
        row.status = "revoked"
        for activation in row.activations:
            activation.revoked_at = datetime.now(UTC).replace(tzinfo=None)
    elif action == "extend":
        row.expires_at = row.expires_at + timedelta(days=days)
    elif action == "reset":
        for activation in row.activations:
            activation.revoked_at = datetime.now(UTC).replace(tzinfo=None)
    else:
        raise HTTPException(status_code=400, detail="Unknown action")
    db.commit()
    _audit(db, f"license.{action}", str(license_id), row.product_id)
    return {"id": row.id, "status": row.status, "expires_at": row.expires_at}


class ReleaseRequest(BaseModel):
    version: str = Field(min_length=5, max_length=24)
    channel: str = "stable"
    platform: str = "windows"
    sha256: str = Field(min_length=64, max_length=64)
    artifact_path: str
    mandatory: bool = False
    security: bool = False
    notes: str = ""


@app.post("/v1/admin/products/{product_id}/releases")
def publish_release(
    product_id: int,
    request: ReleaseRequest,
    db: Session = Depends(get_db),
    x_admin_token: str | None = Header(default=None),
):
    """Publish immediately and sign a product-scoped manifest."""
    from .crypto import make_lease

    _require_session(x_admin_token)
    payload = {
        "version": request.version,
        "channel": request.channel,
        "platform": request.platform,
        "sha256": request.sha256,
        "mandatory": request.mandatory,
        "security": request.security,
        "notes": request.notes,
    }
    signature = make_lease(payload, key_from_env("UPDATE_SIGNING_PRIVATE_KEY"))
    row = Release(
        product_id=product_id,
        version=request.version,
        channel=request.channel,
        platform=request.platform,
        sha256=request.sha256,
        artifact_path=request.artifact_path,
        mandatory=request.mandatory,
        security=request.security,
        manifest_signature=signature,
        status="published",
        notes=request.notes,
    )
    db.add(row)
    db.commit()
    _audit(db, "release.publish", request.version, product_id, channel=request.channel)
    return {"id": row.id, "version": row.version, "signature": signature}


@app.get("/v1/admin/audit")
def audit(limit: int = 100, db: Session = Depends(get_db), x_admin_token: str | None = Header(default=None)):
    _require_session(x_admin_token)
    rows = db.query(AuditEvent).order_by(AuditEvent.created_at.desc()).limit(min(limit, 500))
    return [
        {"id": r.id, "actor": r.actor, "action": r.action, "target": r.target,
         "metadata": json.loads(r.metadata_json or "{}"), "created_at": r.created_at}
        for r in rows
    ]


@app.get("/health")
def health():
    return {"status": "ok"}
