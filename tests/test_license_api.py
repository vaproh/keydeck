"""Public + admin licence API behaviour (R5)."""

import base64
import os
import sys
from datetime import datetime, timedelta, UTC
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "license-server"))


def _b64(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


@pytest.fixture(scope="module")
def env(tmp_path_factory):
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

    db = tmp_path_factory.mktemp("license") / "test.db"
    os.environ["LICENSE_DB_URL"] = f"sqlite:///{db}"
    os.environ["LICENSE_SIGNING_PRIVATE_KEY"] = _b64(
        Ed25519PrivateKey.generate().private_bytes_raw()
    )
    os.environ["UPDATE_SIGNING_PRIVATE_KEY"] = _b64(
        Ed25519PrivateKey.generate().private_bytes_raw()
    )
    os.environ["LICENSE_ADMIN_TOTP_SECRET"] = "JBSWY3DPEHPK3PXP"
    os.environ["LICENSE_DOWNLOAD_SECRET"] = "test-secret"
    return db


@pytest.fixture
def client(env):
    from lsapp.api_public import app as public_app
    from lsapp.db import Base, engine

    Base.metadata.create_all(engine)
    return TestClient(public_app)


@pytest.fixture
def admin():
    from lsapp.api_admin import app as admin_app

    return TestClient(admin_app)


def _make_product_and_key(admin, client):
    import pyotp
    from argon2 import PasswordHasher

    os.environ["LICENSE_ADMIN_PASSWORD_HASH"] = PasswordHasher().hash("admin-pass")
    login = admin.post(
        "/v1/admin/login",
        json={"password": "admin-pass", "totp": pyotp.TOTP("JBSWY3DPEHPK3PXP").now()},
    )
    assert login.status_code == 200
    token = login.json()["token"]
    headers = {"X-Admin-Token": token}

    product = admin.post("/v1/admin/products", json={"slug": "reorchestra", "name": "ReOrchestra"}, headers=headers).json()
    admin.post(f"/v1/admin/products/{product['id']}/keys", headers=headers)
    created = admin.post(
        f"/v1/admin/products/{product['id']}/licenses",
        json={"plan_name": "operator", "limits": {"max_accounts": 12}, "features": ["warmup"]},
        headers=headers,
    )
    assert created.status_code == 200
    return product, created.json(), headers


HASH = "a" * 64


def test_activation_returns_a_verifiable_lease(client, admin):
    from lsapp.crypto import key_from_env, public_key_of, verify_lease

    _product, license_row, _headers = _make_product_and_key(admin, client)
    response = client.post(
        "/v1/activate",
        json={"key": license_row["key"], "installation_hash": HASH, "app_version": "1.0.0"},
    )
    assert response.status_code == 200
    body = response.json()
    claims = verify_lease(body["lease"], public_key_of(key_from_env("LICENSE_SIGNING_PRIVATE_KEY")))
    assert claims["plan_name"] == "operator"
    assert claims["limits"] == {"max_accounts": 12}
    assert claims["features"] == ["warmup"]
    assert claims["installation_hash"] == HASH
    assert body["features"] == ["warmup"]


def test_activation_limit_and_revocation(client, admin):
    product, license_row, headers = _make_product_and_key(admin, client)
    first = client.post("/v1/activate", json={"key": license_row["key"], "installation_hash": HASH})
    assert first.status_code == 200

    other = client.post("/v1/activate", json={"key": license_row["key"], "installation_hash": "b" * 64})
    assert other.status_code == 409

    assert admin.post(f"/v1/admin/licenses/{license_row['id']}/reset", headers=headers).status_code == 200
    assert client.post("/v1/activate", json={"key": license_row["key"], "installation_hash": "b" * 64}).status_code == 200

    assert admin.post(f"/v1/admin/licenses/{license_row['id']}/revoke", headers=headers).status_code == 200
    revoked = client.post("/v1/activate", json={"key": license_row["key"], "installation_hash": "c" * 64})
    assert revoked.status_code == 401


def test_unknown_key_is_rejected(client):
    assert client.post("/v1/activate", json={"key": "reo_nope", "installation_hash": HASH}).status_code == 401


def test_expired_key_is_rejected(client, admin):
    from lsapp.db import SessionLocal
    from lsapp.models import License

    _product, license_row, _headers = _make_product_and_key(admin, client)
    db = SessionLocal()
    try:
        row = db.query(License).filter(License.id == license_row["id"]).first()
        row.expires_at = datetime.now(UTC).replace(tzinfo=None) - timedelta(days=1)
        db.commit()
    finally:
        db.close()
    assert client.post("/v1/activate", json={"key": license_row["key"], "installation_hash": HASH}).status_code == 403


def test_admin_requires_password_and_totp(admin):
    from argon2 import PasswordHasher

    os.environ["LICENSE_ADMIN_PASSWORD_HASH"] = PasswordHasher().hash("admin-pass")
    assert admin.post("/v1/admin/login", json={"password": "wrong", "totp": "123456"}).status_code == 401
    assert admin.post("/v1/admin/products", json={"slug": "xy", "name": "x"}).status_code == 401


def test_public_app_has_no_admin_routes(client):
    assert client.get("/v1/admin/products").status_code == 404
