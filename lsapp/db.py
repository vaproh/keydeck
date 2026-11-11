"""Database wiring shared by the public API and the private admin."""

from __future__ import annotations

import os

from sqlalchemy import create_engine, inspect, text
from sqlalchemy.orm import DeclarativeBase, sessionmaker


def database_url() -> str:
    return os.getenv("LICENSE_DB_URL", "sqlite:///./license.db")


class Base(DeclarativeBase):
    pass


engine = create_engine(database_url(), future=True)
SessionLocal = sessionmaker(bind=engine, autoflush=False, expire_on_commit=False)


def _add_missing_license_columns() -> None:
    """Lightweight additive migration for existing deployments.

    ``create_all`` never alters an existing table, so new key fields must be
    added explicitly. Only nullable/defaulted columns are added here.
    """
    inspector = inspect(engine)
    if "licenses" not in inspector.get_table_names():
        return
    existing = {column["name"] for column in inspector.get_columns("licenses")}
    boolean_default = "FALSE" if engine.dialect.name == "postgresql" else "0"
    columns = {
        "key_type": "VARCHAR(16) NOT NULL DEFAULT 'timed'",
        "trial_minutes": "INTEGER",
        "validity_days": "INTEGER",
        "starts_on_activation": f"BOOLEAN NOT NULL DEFAULT {boolean_default}",
        "one_time_activation": f"BOOLEAN NOT NULL DEFAULT {boolean_default}",
        "grace_seconds": "INTEGER NOT NULL DEFAULT 86400",
        "first_activated_at": "TIMESTAMP",
        "activation_expires_at": "TIMESTAMP",
        "used_at": "TIMESTAMP",
    }
    with engine.begin() as connection:
        for name, definition in columns.items():
            if name not in existing:
                connection.execute(
                    text(f"ALTER TABLE licenses ADD COLUMN {name} {definition}")
                )


def init_db() -> None:
    """Create tables and add new key columns to an existing database."""
    from . import models  # noqa: F401  (register mappers before create_all)

    Base.metadata.create_all(engine)
    _add_missing_license_columns()


def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
