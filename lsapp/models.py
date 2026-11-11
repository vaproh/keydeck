"""Product-scoped licensing: no pricing, tiers, billing or customers."""

from __future__ import annotations

from datetime import datetime, UTC

from sqlalchemy import DateTime, ForeignKey, Integer, String, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from .db import Base


def _now() -> datetime:
    return datetime.now(UTC).replace(tzinfo=None)


class Product(Base):
    __tablename__ = "products"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    slug: Mapped[str] = mapped_column(String(40), unique=True, index=True)
    name: Mapped[str] = mapped_column(String(120))
    status: Mapped[str] = mapped_column(String(16), default="active")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=_now)


class ProductKey(Base):
    """Per-product signing keys; private halves live in the environment."""

    __tablename__ = "product_keys"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    product_id: Mapped[int] = mapped_column(ForeignKey("products.id"), index=True)
    license_public_key: Mapped[str] = mapped_column(String(128))
    update_public_key: Mapped[str] = mapped_column(String(128))
    key_prefix: Mapped[str] = mapped_column(String(12), default="reo")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=_now)


class License(Base):
    __tablename__ = "licenses"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    product_id: Mapped[int] = mapped_column(ForeignKey("products.id"), index=True)
    key_hash: Mapped[str] = mapped_column(String(64), unique=True, index=True)
    key_last4: Mapped[str] = mapped_column(String(8))
    plan_name: Mapped[str] = mapped_column(String(40), default="standard")
    limits_json: Mapped[str] = mapped_column(Text, default="{}")
    features_json: Mapped[str] = mapped_column(Text, default="[]")
    expires_at: Mapped[datetime] = mapped_column(DateTime)
    max_activations: Mapped[int] = mapped_column(Integer, default=1)
    status: Mapped[str] = mapped_column(String(16), default="active")
    notes: Mapped[str] = mapped_column(Text, default="")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=_now)

    activations: Mapped[list["Activation"]] = relationship(back_populates="license")


class Activation(Base):
    __tablename__ = "activations"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    license_id: Mapped[int] = mapped_column(ForeignKey("licenses.id"), index=True)
    installation_hash: Mapped[str] = mapped_column(String(64), index=True)
    machine_hash: Mapped[str] = mapped_column(String(64), default="")
    app_version: Mapped[str] = mapped_column(String(24), default="")
    last_seen_at: Mapped[datetime] = mapped_column(DateTime, default=_now)
    ip: Mapped[str] = mapped_column(String(64), default="")
    revoked_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)

    license: Mapped[License] = relationship(back_populates="activations")


class Release(Base):
    __tablename__ = "releases"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    product_id: Mapped[int] = mapped_column(ForeignKey("products.id"), index=True)
    version: Mapped[str] = mapped_column(String(24), index=True)
    channel: Mapped[str] = mapped_column(String(12), default="stable")
    platform: Mapped[str] = mapped_column(String(16), default="windows")
    min_version: Mapped[str] = mapped_column(String(24), default="0.0.0")
    mandatory: Mapped[bool] = mapped_column(default=False)
    security: Mapped[bool] = mapped_column(default=False)
    rollout_percent: Mapped[int] = mapped_column(Integer, default=100)
    sha256: Mapped[str] = mapped_column(String(64))
    artifact_path: Mapped[str] = mapped_column(String(300))
    manifest_signature: Mapped[str] = mapped_column(Text, default="")
    status: Mapped[str] = mapped_column(String(16), default="draft")
    notes: Mapped[str] = mapped_column(Text, default="")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=_now)


class AuditEvent(Base):
    __tablename__ = "audit_events"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    product_id: Mapped[int | None] = mapped_column(Integer, nullable=True)
    actor: Mapped[str] = mapped_column(String(80), default="admin")
    action: Mapped[str] = mapped_column(String(60))
    target: Mapped[str] = mapped_column(String(120), default="")
    metadata_json: Mapped[str] = mapped_column(Text, default="{}")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=_now)
