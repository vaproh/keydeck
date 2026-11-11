"""Lease signing and key helpers shared by both services."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import secrets
from datetime import datetime, timedelta, UTC

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import (
    Ed25519PrivateKey,
    Ed25519PublicKey,
)

LEASE_HOURS = int(os.getenv("LICENSE_LEASE_HOURS", "24"))


def b64decode(value: str) -> bytes:
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


def b64encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).decode().rstrip("=")


def key_from_env(name: str) -> Ed25519PrivateKey:
    raw = os.getenv(name)
    if not raw:
        raise RuntimeError(f"{name} is not configured")
    return Ed25519PrivateKey.from_private_bytes(b64decode(raw))


def public_key_of(private: Ed25519PrivateKey) -> str:
    raw = private.public_key().public_bytes(
        serialization.Encoding.Raw, serialization.PublicFormat.Raw
    )
    return b64encode(raw)


def sha256_hex(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()


def new_key(prefix: str) -> str:
    return f"{prefix}_{secrets.token_urlsafe(24)}"


def make_lease(claims: dict, key: Ed25519PrivateKey) -> str:
    payload = b64encode(json.dumps(claims, separators=(",", ":")).encode())
    signature = b64encode(key.sign(payload.encode()))
    return f"{payload}.{signature}"


def verify_lease(token: str, public_key: str) -> dict:
    payload, signature = token.split(".", 1)
    raw = b64decode(public_key)
    Ed25519PublicKey.from_public_bytes(raw).verify(b64decode(signature), payload.encode())
    return json.loads(b64decode(payload))


def lease_claims(license_row, installation_hash: str, limits: dict, features: list[str]) -> dict:
    now = datetime.now(UTC)
    expires = min(license_row.expires_at.replace(tzinfo=UTC), now + timedelta(hours=LEASE_HOURS))
    grace_hours = int(os.getenv("LICENSE_OFFLINE_GRACE_HOURS", "24"))
    return {
        "product": license_row.product.slug if hasattr(license_row, "product") else "",
        "license_id": license_row.id,
        "plan_name": license_row.plan_name,
        "limits": limits,
        "features": features,
        "installation_hash": installation_hash,
        "issued_at": now.timestamp(),
        "expires_at": expires.timestamp(),
        "grace_until": (expires + timedelta(hours=grace_hours)).timestamp(),
    }
