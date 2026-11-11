"""Database wiring shared by the public API and the private admin."""

from __future__ import annotations

import os

from sqlalchemy import create_engine
from sqlalchemy.orm import DeclarativeBase, sessionmaker


def database_url() -> str:
    return os.getenv("LICENSE_DB_URL", "sqlite:///./license.db")


class Base(DeclarativeBase):
    pass


engine = create_engine(database_url(), future=True)
SessionLocal = sessionmaker(bind=engine, autoflush=False, expire_on_commit=False)


def init_db() -> None:
    """Create tables on startup so a fresh deployment never serves 500s."""
    from . import models  # noqa: F401  (register mappers before create_all)

    Base.metadata.create_all(engine)


def get_db():
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
