"""Public licence API: activation, heartbeat, updates, downloads.

No admin routes exist here, so a leaked public URL cannot mint keys.
"""

from __future__ import annotations

import hashlib
import hmac
import json
import os
from datetime import datetime, UTC
from pathlib import Path

from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field
from sqlalchemy.orm import Session

from .crypto import lease_claims, make_lease, product_signing_key, sha256_hex
from .db import get_db, init_db
from .models import Activation, License, Product, Release

from contextlib import asynccontextmanager


@asynccontextmanager
async def lifespan(_: FastAPI):
    init_db()
    yield


app = FastAPI(title="ReOrchestra license API", version="1.0", lifespan=lifespan)


DOWNLOAD_TTL = 900


class ActivateRequest(BaseModel):
    key: str = Field(min_length=8, max_length=128)
    installation_hash: str = Field(pattern=r"^[a-f0-9]{64}$")
    machine_hash: str = Field(default="", max_length=64)
    app_version: str = Field(default="", max_length=24)
    product: str = Field(default="reorchestra", max_length=40)


class HeartbeatRequest(ActivateRequest):
    pass


def _license_for(db: Session, key: str, product_slug: str) -> License:
    row = db.query(License).filter(License.key_hash == sha256_hex(key)).first()
    if not row or row.status != "active":
        raise HTTPException(status_code=401, detail="Invalid license")
    product = db.query(Product).filter(Product.id == row.product_id).first()
    if not product or product.slug != product_slug:
        # A key issued for one product must never unlock another.
        raise HTTPException(status_code=403, detail="License is for another product")
    if row.expires_at <= datetime.now(UTC).replace(tzinfo=None):
        raise HTTPException(status_code=403, detail="License expired")
    return row


def _activate(db: Session, request: ActivateRequest, ip: str) -> dict:
    row = _license_for(db, request.key, request.product)
    rows = [a for a in row.activations if a.revoked_at is None]
    existing = next((a for a in rows if a.installation_hash == request.installation_hash), None)
    if existing is None:
        if len(rows) >= row.max_activations:
            raise HTTPException(status_code=409, detail="Activation limit reached")
        existing = Activation(
            license_id=row.id,
            installation_hash=request.installation_hash,
            machine_hash=request.machine_hash,
            app_version=request.app_version,
            ip=ip,
        )
        db.add(existing)
    else:
        existing.last_seen_at = datetime.now(UTC).replace(tzinfo=None)
        existing.app_version = request.app_version
    db.commit()

    limits = json.loads(row.limits_json or "{}")
    features = json.loads(row.features_json or "[]")
    product = db.query(Product).filter(Product.id == row.product_id).first()
    if not product:
        raise HTTPException(status_code=500, detail="Product misconfigured")
    private = product_signing_key(product.slug, "license")
    claims = lease_claims(
        row, request.installation_hash, limits, features, product.slug
    )
    return {"lease": make_lease(claims, private), "plan_name": row.plan_name, "limits": limits, "features": features}


@app.post("/v1/activate")
def activate(request: ActivateRequest, http: Request, db: Session = Depends(get_db)):
    return _activate(db, request, http.client.host if http.client else "")


@app.post("/v1/heartbeat")
def heartbeat(request: HeartbeatRequest, db: Session = Depends(get_db)):
    return _activate(db, request, "")


@app.get("/v1/update-check")
def update_check(
    product: str,
    channel: str = "stable",
    current: str = "0.0.0",
    db: Session = Depends(get_db),
):
    prod = db.query(Product).filter(Product.slug == product).first()
    if not prod:
        raise HTTPException(status_code=404, detail="Unknown product")
    row = (
        db.query(Release)
        .filter(
            Release.product_id == prod.id,
            Release.channel == channel,
            Release.status == "published",
        )
        .order_by(Release.created_at.desc())
        .first()
    )
    if not row:
        return {"available": False}

    manifest = json.loads(row.manifest_json or "{}")
    if not manifest and row.manifest_signature:
        manifest = {
            "product": prod.slug,
            "version": row.version,
            "channel": row.channel,
            "platform": row.platform,
            "min_version": row.min_version,
            "mandatory": row.mandatory,
            "security": row.security,
            "sha256": row.sha256,
            "notes": row.notes,
        }

    # The download token is short-lived and bound to this release and product.
    expires = int(datetime.now(UTC).timestamp()) + DOWNLOAD_TTL
    token = _download_token(row.id, prod.slug, expires)
    base = os.getenv("PUBLIC_BASE_URL", "").rstrip("/")
    download_url = (
        f"{base}/v1/download/{row.id}?token={token}&exp={expires}&product={prod.slug}"
    )
    return {
        "available": row.version != current,
        "manifest": manifest,
        "signature": row.manifest_signature,
        # Outside the signed body: the token changes on every check.
        "download_url": download_url,
        "expires_at": expires,
    }


def _download_token(release_id: int, product_slug: str, expires: int) -> str:
    secret = os.getenv("LICENSE_DOWNLOAD_SECRET", "dev-secret").encode()
    return hmac.new(
        secret, f"{release_id}:{product_slug}:{expires}".encode(), hashlib.sha256
    ).hexdigest()[:32]


@app.get("/v1/download/{release_id}")
def download(
    release_id: int,
    token: str,
    exp: int = 0,
    product: str = "",
    db: Session = Depends(get_db),
):
    row = db.query(Release).filter(Release.id == release_id, Release.status == "published").first()
    if not row:
        raise HTTPException(status_code=404, detail="Release not found")
    owner = db.query(Product).filter(Product.id == row.product_id).first()
    if not owner or owner.slug != product:
        raise HTTPException(status_code=403, detail="Token is for another product")
    if exp and exp < int(datetime.now(UTC).timestamp()):
        raise HTTPException(status_code=403, detail="Download token expired")
    expected = _download_token(row.id, owner.slug, exp)
    if len(token) != 32 or not hmac.compare_digest(token, expected):
        raise HTTPException(status_code=403, detail="Invalid download token")
    path = Path(row.artifact_path)
    if not path.exists():
        raise HTTPException(status_code=404, detail="Artifact missing")
    return FileResponse(
        path,
        media_type="application/zip",
        filename=f"{owner.slug}-{row.version}.zip",
        headers={"X-Checksum-Sha256": row.sha256},
    )


@app.get("/health")
def health():
    return {"status": "ok"}
