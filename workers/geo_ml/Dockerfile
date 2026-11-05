FROM python:3.12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends git libexpat1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work
COPY workers/geo_ml/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu torch==2.9.0+cpu \
    && grep -v '^torch==' /tmp/requirements.txt > /tmp/requirements-no-torch.txt \
    && pip install --no-cache-dir -r /tmp/requirements-no-torch.txt

ENTRYPOINT ["python"]
