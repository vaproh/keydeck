FROM python:3.12-slim
WORKDIR /srv
RUN pip install --no-cache-dir uv
COPY pyproject.toml uv.lock ./
COPY license-server ./license-server
RUN uv sync --frozen --group license --no-dev
ENV PATH="/srv/.venv/bin:$PATH"
WORKDIR /srv/license-server
EXPOSE 8000
