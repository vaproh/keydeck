FROM python:3.12-slim
WORKDIR /srv
COPY --from=ghcr.io/astral-sh/uv:0.9 /uv /uvx /bin/
COPY pyproject.toml uv.lock ./
COPY lsapp ./lsapp
COPY templates ./templates
COPY static ./static
RUN uv sync --frozen --no-dev
ENV PATH="/srv/.venv/bin:$PATH"
EXPOSE 8000
