"""Public licence API and admin panel behaviour (I2)."""

import base64
import os
import re
import sys
from datetime import datetime, timedelta, UTC
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "license-server"))

HASH = "a" * 64
TOTP_SECRET = "JBSWY3DPEHPK3PXP"
PASSWORD = "admin-pass"


def _b64(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _new_key() -> str:
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

    return _b64(Ed25519PrivateKey.generate().private_bytes_raw())


@pytest.fixture(scope="module")
def env(tmp_path_factory):
    db = tmp_path_factory.mktemp("license") / "test.db"
    os.environ["LICENSE_DB_URL"] = f"sqlite:///{db}"
    os.environ["LICENSE_SIGNING_KEY_REORCHESTRA"] = _new_key()
    os.environ["UPDATE_SIGNING_KEY_REORCHESTRA"] = _new_key()
    os.environ["LICENSE_SIGNING_KEY_OTHER_APP"] = _new_key()
    os.environ["UPDATE_SIGNING_KEY_OTHER_APP"] = _new_key()
    os.environ["LICENSE_ADMIN_TOTP_SECRET"] = TOTP_SECRET
    os.environ["LICENSE_DOWNLOAD_SECRET"] = "test-secret"
    os.environ["ADMIN_SERVICE_TOKEN"] = "ci-service-token"
    os.environ["ADMIN_COOKIE_SECURE"] = "0"
    os.environ["PUBLIC_BASE_URL"] = "https://license.test"
    os.environ["LICENSE_ARTIFACTS_DIR"] = str(db.parent / "artifacts")
    return db


@pytest.fixture(autouse=True)
def reset_admin_state(env):
    """Sessions, lockouts and DB rows are module state; isolate every test."""
    from lsapp import api_admin
    from lsapp.db import Base, engine

    Base.metadata.create_all(engine)
    api_admin.SESSIONS.clear()
    api_admin._FAILURES.clear()
    yield
    api_admin.SESSIONS.clear()
    api_admin._FAILURES.clear()


@pytest.fixture
def public(env):
    from lsapp.api_public import app
    from lsapp.db import Base, engine

    Base.metadata.create_all(engine)
    return TestClient(app)


@pytest.fixture
def admin_app(env):
    return TestClient


def _csrf(html: str) -> str:
    match = re.search(r'name="csrf_token" value="([^"]+)"', html)
    assert match, "csrf token missing"
    return match.group(1)


def login(admin):
    from argon2 import PasswordHasher
    import pyotp

    os.environ["LICENSE_ADMIN_PASSWORD_HASH"] = PasswordHasher().hash(PASSWORD)
    response = admin.post(
        "/admin/login",
        data={"password": PASSWORD, "totp": pyotp.TOTP(TOTP_SECRET).now()},
        follow_redirects=False,
    )
    assert response.status_code == 303
    page = admin.get("/admin")
    assert page.status_code == 200
    return _csrf(page.text)


def make_product(admin, csrf, slug="reorchestra"):
    response = admin.post(
        "/admin/products", data={"csrf_token": csrf, "slug": slug, "name": slug.title()},
        follow_redirects=False,
    )
    assert response.status_code == 303
    from lsapp.db import SessionLocal
    from lsapp.models import Product

    db = SessionLocal()
    try:
        product_id = db.query(Product).filter(Product.slug == slug).one().id
    finally:
        db.close()
    keys = admin.post(
        f"/admin/products/{product_id}/keys",
        data={"csrf_token": csrf, "prefix": "reo"},
        follow_redirects=False,
    )
    assert keys.status_code == 303
    return product_id


def make_license(admin, csrf, product_id, **overrides):
    data = {
        "csrf_token": csrf,
        "plan_name": "operator",
        "days": "30",
        "max_activations": "1",
        "limits": '{"accounts": 12}',
        "features": '["warmup"]',
        "notes": "test",
        **overrides,
    }
    response = admin.post(
        f"/admin/products/{product_id}/licenses", data=data, follow_redirects=False
    )
    assert response.status_code == 200
    key = re.search(r'class="key">([^<]+)<', response.text).group(1)
    return key


# --------------------------------------------------------------- auth tests


def test_login_success_sets_a_session_cookie(admin_app):
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    login(admin)
    cookie = admin.cookies.get("reo_admin")
    assert cookie
    # The cookie is HttpOnly and SameSite=strict per the Set-Cookie header.
    header = admin.get("/admin").headers
    assert header["content-type"].startswith("text/html")


def test_login_rejects_wrong_password_and_totp(admin_app):
    from argon2 import PasswordHasher

    os.environ["LICENSE_ADMIN_PASSWORD_HASH"] = PasswordHasher().hash(PASSWORD)
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    bad_password = admin.post(
        "/admin/login", data={"password": "nope", "totp": "123456"}, follow_redirects=False
    )
    assert bad_password.status_code == 401

    bad_totp = admin.post(
        "/admin/login",
        data={"password": PASSWORD, "totp": "000000"},
        follow_redirects=False,
    )
    assert bad_totp.status_code == 401


def test_login_is_rate_limited(admin_app):
    from argon2 import PasswordHasher

    os.environ["LICENSE_ADMIN_PASSWORD_HASH"] = PasswordHasher().hash(PASSWORD)
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    for _ in range(5):
        admin.post("/admin/login", data={"password": "x", "totp": "1"}, follow_redirects=False)
    blocked = admin.post(
        "/admin/login", data={"password": "x", "totp": "1"}, follow_redirects=False
    )
    assert blocked.status_code == 429


def test_session_expiry_forces_login(admin_app):
    from lsapp import api_admin

    admin = admin_app(api_admin.app)
    login(admin)
    token = admin.cookies.get("reo_admin")
    api_admin.SESSIONS[token].expires_at = datetime.now(UTC) - timedelta(minutes=1)
    page = admin.get("/admin", follow_redirects=False)
    assert page.status_code == 303
    assert page.headers["location"] == "/admin/login"


def test_unauthenticated_panel_redirects_to_login(admin_app):
    from lsapp import api_admin

    admin = admin_app(api_admin.app)
    response = admin.get("/admin", follow_redirects=False)
    assert response.status_code == 303
    assert response.headers["location"] == "/admin/login"
    assert admin.get("/admin/licenses", follow_redirects=False).status_code == 303


def test_missing_or_wrong_csrf_is_rejected(admin_app):
    from lsapp import api_admin

    admin = admin_app(api_admin.app)
    csrf = login(admin)
    assert admin.post("/admin/products", data={"slug": "x", "name": "x"}, follow_redirects=False).status_code == 403
    assert (
        admin.post(
            "/admin/products",
            data={"csrf_token": "wrong", "slug": "x", "name": "x"},
            follow_redirects=False,
        ).status_code
        == 403
    )
    # A correct token still works.
    assert (
        admin.post(
            "/admin/products",
            data={"csrf_token": csrf, "slug": "csrf-ok", "name": "CSRF ok"},
            follow_redirects=False,
        ).status_code
        == 303
    )


# ------------------------------------------------------ keys and signing keys


def test_per_product_signing_keys_are_used(admin_app):
    from lsapp.crypto import product_signing_key, public_key_of
    from lsapp.db import SessionLocal
    from lsapp.models import ProductKey

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf, "other-app")
    db = SessionLocal()
    try:
        row = db.query(ProductKey).order_by(ProductKey.id.desc()).first()
        assert row.license_key_ref == "LICENSE_SIGNING_KEY_OTHER_APP"
        assert row.update_key_ref == "UPDATE_SIGNING_KEY_OTHER_APP"
        assert row.license_public_key == public_key_of(
            product_signing_key("other-app", "license")
        )
        assert row.license_public_key != public_key_of(
            product_signing_key("reorchestra", "license")
        )
    finally:
        db.close()


def test_license_actions_extend_reset_revoke_and_audit(admin_app):
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    product_id = make_product(admin, csrf)
    key = make_license(admin, csrf, product_id)

    from lsapp.db import SessionLocal
    from lsapp.models import AuditEvent, License

    db = SessionLocal()
    try:
        row = db.query(License).order_by(License.id.desc()).first()
        before = row.expires_at
    finally:
        db.close()

    assert admin.post(
        f"/admin/licenses/{row.id}/extend",
        data={"csrf_token": csrf, "days": "10"},
        follow_redirects=False,
    ).status_code == 303
    db = SessionLocal()
    try:
        assert db.get(License, row.id).expires_at > before
    finally:
        db.close()

    assert admin.post(
        f"/admin/licenses/{row.id}/reset", data={"csrf_token": csrf}, follow_redirects=False
    ).status_code == 303
    assert admin.post(
        f"/admin/licenses/{row.id}/revoke", data={"csrf_token": csrf}, follow_redirects=False
    ).status_code == 303

    db = SessionLocal()
    try:
        assert db.get(License, row.id).status == "revoked"
        actions = {event.action for event in db.query(AuditEvent).all()}
        assert {"license.issue", "license.extend", "license.reset", "license.revoke"} <= actions
    finally:
        db.close()
    assert key


# ------------------------------------------------------------- public API tests


def test_activation_uses_the_product_key_and_enforces_limits(admin_app, public):
    from lsapp.crypto import product_signing_key, public_key_of, verify_lease

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    product_id = make_product(admin, csrf)
    key = make_license(admin, csrf, product_id, max_activations="1")

    first = public.post("/v1/activate", json={"key": key, "installation_hash": HASH})
    assert first.status_code == 200
    claims = verify_lease(
        first.json()["lease"], public_key_of(product_signing_key("reorchestra", "license"))
    )
    assert claims["plan_name"] == "operator"
    assert claims["limits"] == {"accounts": 12}

    assert public.post("/v1/activate", json={"key": key, "installation_hash": "b" * 64}).status_code == 409


def test_unknown_and_expired_keys_are_rejected(admin_app, public):
    assert public.post("/v1/activate", json={"key": "reo_nope", "installation_hash": HASH}).status_code == 401

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    product_id = make_product(admin, csrf)
    key = make_license(admin, csrf, product_id)

    from lsapp.db import SessionLocal
    from lsapp.models import License

    db = SessionLocal()
    try:
        row = db.query(License).order_by(License.id.desc()).first()
        row.expires_at = datetime.now(UTC).replace(tzinfo=None) - timedelta(days=1)
        db.commit()
    finally:
        db.close()
    assert public.post("/v1/activate", json={"key": key, "installation_hash": HASH}).status_code == 403


# ------------------------------------------------------------- service token


def test_service_token_publishes_releases_only(admin_app, public):
    import hashlib

    from lsapp.crypto import product_signing_key, public_key_of
    from app.modules.ota import verify_manifest

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)

    payload = b"PKzip-bytes"
    form = {
        "version": "1.2.0",
        "channel": "stable",
        "platform": "windows",
        "min_version": "1.0.0",
        "mandatory": "false",
        "security": "false",
        "notes": "ci",
        "sha256": hashlib.sha256(payload).hexdigest(),
    }
    files = {"artifact": ("app.zip", payload, "application/zip")}
    url = "/v1/admin/products/reorchestra/releases"

    assert admin.post(url, data=form, files=files).status_code == 401
    # A human session cookie must not authenticate the CI endpoint.
    assert (
        admin.post(url, data=form, files=files, cookies=admin.cookies).status_code == 401
    )

    created = admin.post(
        url, data=form, files=files, headers={"X-Service-Token": "ci-service-token"}
    )
    assert created.status_code == 200, created.text
    assert created.json()["status"] == "published"

    check = public.get(
        "/v1/update-check",
        params={"product": "reorchestra", "channel": "stable", "current": "1.0.0"},
    ).json()
    verify_manifest(
        check["manifest"],
        check["signature"],
        public_key_of(product_signing_key("reorchestra", "update")),
    )
    assert check["manifest"]["version"] == "1.2.0"
    assert check["manifest"]["product"] == "reorchestra"


def test_public_app_has_no_admin_routes(public):
    assert public.get("/v1/admin/products").status_code == 404
    assert public.get("/admin").status_code == 404


# ------------------------------------------------------- product scoping (I3)


def _activate(public, key, product="reorchestra", installation=HASH):
    return public.post(
        "/v1/activate",
        json={
            "key": key,
            "installation_hash": installation,
            "machine_hash": "b" * 64,
            "app_version": "1.0.0",
            "product": product,
        },
    )


def test_activation_is_product_scoped(admin_app):
    from lsapp.crypto import product_signing_key, verify_lease, public_key_of
    from lsapp.api_public import app as public_app

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    product_id = make_product(admin, csrf)
    key = make_license(admin, csrf, product_id)
    public = TestClient(public_app)

    ok = _activate(public, key)
    assert ok.status_code == 200, ok.text
    claims = verify_lease(
        ok.json()["lease"],
        public_key_of(product_signing_key("reorchestra", "license")),
    )
    assert claims["product"] == "reorchestra"

    wrong = _activate(public, key, product="other-app")
    assert wrong.status_code == 403
    assert "another product" in wrong.json()["detail"]


def test_activation_rejects_a_key_from_another_product(admin_app):
    from lsapp.api_public import app as public_app

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)
    other_id = make_product(admin, csrf, slug="other-app")
    other_key = make_license(admin, csrf, other_id)

    public = TestClient(public_app)
    assert _activate(public, other_key, product="reorchestra").status_code == 403
    assert _activate(public, other_key, product="other-app").status_code == 200


# -------------------------------------------------------------- releases (I3)


def _upload(public_base, slug="reorchestra", version="1.2.0", channel="stable", payload=b"PKzip", digest=None, token="ci-service-token"):
    import hashlib

    from lsapp.api_admin import app as admin_app

    digest = digest or hashlib.sha256(payload).hexdigest()
    return TestClient(admin_app).post(
        f"/v1/admin/products/{slug}/releases",
        headers={"X-Service-Token": token} if token else {},
        data={
            "version": version,
            "channel": channel,
            "platform": "windows",
            "min_version": "1.0.0",
            "mandatory": "false",
            "security": "false",
            "notes": "automated",
            "sha256": digest,
        },
        files={"artifact": ("app.zip", payload, "application/zip")},
    )


def test_release_upload_signs_a_canonical_manifest(admin_app):
    import json

    from lsapp.crypto import public_key_of, product_signing_key
    from app.modules.ota import verify_manifest

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)

    response = _upload(None)
    assert response.status_code == 200, response.text

    public = TestClient(__import__("lsapp.api_public", fromlist=["app"]).app)
    check = public.get(
        "/v1/update-check",
        params={"product": "reorchestra", "channel": "stable", "current": "1.0.0"},
    ).json()
    assert check["available"] is True
    manifest = check["manifest"]
    assert manifest["version"] == "1.2.0"
    assert check["download_url"].startswith("https://license.test/v1/download/")
    verify_manifest(
        manifest,
        check["signature"],
        public_key_of(product_signing_key("reorchestra", "update")),
    )
    assert json.dumps(manifest, sort_keys=True)  # canonical-safe


def test_release_upload_rejects_a_checksum_mismatch(admin_app, tmp_path):
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)
    response = _upload(None, version="1.3.0", digest="0" * 64)
    assert response.status_code == 400
    assert "Checksum" in response.json()["detail"]
    assert not (tmp_path / "x").exists()


def test_download_returns_zip_bytes_and_checks_token(admin_app):
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)
    _upload(None, version="1.4.0", payload=b"PK\x03\x04data")

    public = TestClient(__import__("lsapp.api_public", fromlist=["app"]).app)
    check = public.get(
        "/v1/update-check", params={"product": "reorchestra", "current": "1.0.0"}
    ).json()
    url = check["download_url"].replace("https://license.test", "")

    good = public.get(url)
    assert good.status_code == 200
    assert good.headers["content-type"] == "application/zip"
    assert good.content == b"PK\x03\x04data"
    assert good.headers["x-checksum-sha256"]

    assert public.get(url + "&product=other-app").status_code == 403
    expired = url.split("&exp=")[0] + "&exp=1&product=reorchestra"
    assert public.get(expired).status_code == 403


def test_release_upload_requires_the_service_token(admin_app):
    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)
    assert _upload(None, token="").status_code == 401
    assert _upload(None, token="wrong").status_code == 401


def test_release_upload_is_idempotent(admin_app):
    from lsapp.db import SessionLocal
    from lsapp.models import Release

    admin = admin_app(__import__("lsapp.api_admin", fromlist=["app"]).app)
    csrf = login(admin)
    make_product(admin, csrf)

    first = _upload(None, version="1.5.0")
    second = _upload(None, version="1.5.0")
    assert first.status_code == second.status_code == 200
    assert first.json()["id"] == second.json()["id"]

    db = SessionLocal()
    try:
        rows = db.query(Release).filter(Release.version == "1.5.0").all()
    finally:
        db.close()
    assert len(rows) == 1
