FROM python:3.12-slim
WORKDIR /srv
# Copy uv from its official image so the build never depends on pip.
COPY --from=ghcr.io/astral-sh/uv:0.9 /uv /uvx /bin/
COPY pyproject.toml uv.lock ./
COPY license-server ./license-server
RUN uv sync --frozen --group license --no-dev
ENV PATH="/srv/.venv/bin:$PATH"
WORKDIR /srv/license-server
EXPOSE 8000
