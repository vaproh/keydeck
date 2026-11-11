"""Lease signing and key helpers shared by both services."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import secrets
from datetime import UTC, datetime, timedelta

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


def product_env_suffix(slug: str) -> str:
    return slug.upper().replace("-", "_").replace(".", "_")


def product_signing_key(slug: str, kind: str) -> Ed25519PrivateKey:
    """Resolve a per-product signing key; there is no global fallback.

    Env names are `LICENSE_SIGNING_KEY_<SLUG>` and `UPDATE_SIGNING_KEY_<SLUG>`.
    """
    if kind not in {"license", "update"}:
        raise ValueError(f"Unknown key kind: {kind}")
    return key_from_env(f"{kind.upper()}_SIGNING_KEY_{product_env_suffix(slug)}")


def product_signing_env(slug: str, kind: str) -> str:
    return f"{kind.upper()}_SIGNING_KEY_{product_env_suffix(slug)}"


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


def canonical_json(payload: dict) -> str:
    return json.dumps(payload, sort_keys=True, separators=(",", ":"))


def sign_manifest(payload: dict, key: Ed25519PrivateKey) -> str:
    """Sign the canonical body itself so clients can rebuild it byte-for-byte."""
    raw = canonical_json(payload).encode()
    return f"{b64encode(raw)}.{b64encode(key.sign(raw))}"


def verify_manifest(
    token_or_manifest, signature_or_key: str, public_key: str | None = None
) -> dict:
    """Accept either a signed token or a manifest plus detached signature."""
    if public_key is None:
        body, signature = token_or_manifest.split(".", 1)
        raw = b64decode(body)
        key = signature_or_key
    elif "." in signature_or_key:
        body, signature = signature_or_key.split(".", 1)
        raw = b64decode(body)
        key = public_key
    else:
        raw = (
            canonical_json(token_or_manifest).encode()
            if isinstance(token_or_manifest, dict)
            else str(token_or_manifest).encode()
        )
        signature = signature_or_key
        key = public_key
    Ed25519PublicKey.from_public_bytes(b64decode(key)).verify(
        b64decode(signature), raw
    )
    return json.loads(raw)


def effective_expires_at(license_row) -> datetime | None:
    """The server-side expiry for this key, or None for lifetime keys.

    A key that starts its validity on activation has no effective expiry until
    the first activation creates ``activation_expires_at``.
    """
    if getattr(license_row, "key_type", "timed") == "lifetime":
        return None
    if getattr(license_row, "starts_on_activation", False):
        return license_row.activation_expires_at
    return license_row.activation_expires_at or license_row.expires_at


def lease_claims(
    license_row,
    installation_hash: str,
    limits: dict,
    features: list[str],
    product_slug: str = "",
) -> dict:
    now = datetime.now(UTC).replace(tzinfo=None)
    key_type = getattr(license_row, "key_type", "timed")
    lifetime = key_type == "lifetime"
    key_expiry = effective_expires_at(license_row)
    if lifetime:
        lease_expires = now + timedelta(hours=LEASE_HOURS)
    else:
        if key_expiry is None:
            raise ValueError("Key has no effective expiry")
        lease_expires = min(key_expiry, now + timedelta(hours=LEASE_HOURS))
    grace_seconds = 0 if key_type == "trial" else int(
        getattr(license_row, "grace_seconds", 86400) or 0
    )
    return {
        "product": product_slug,
        "license_id": license_row.id,
        "plan_name": license_row.plan_name,
        "key_type": key_type,
        "lifetime": lifetime,
        "limits": limits,
        "features": features,
        "installation_hash": installation_hash,
        "server_now": now.timestamp(),
        "issued_at": now.timestamp(),
        "expires_at": lease_expires.timestamp(),
        "grace_until": (lease_expires + timedelta(seconds=grace_seconds)).timestamp(),
    }
