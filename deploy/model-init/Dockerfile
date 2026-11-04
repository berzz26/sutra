# One-shot model initializer. Kept separate from the API image so the runtime
# image never needs network access to Hugging Face at startup.
FROM alpine:3.20.3

RUN apk add --no-cache ca-certificates curl jq

COPY deploy/model-init/fetch-model.sh /usr/local/bin/fetch-model
COPY deploy/model-init/model-manifest.json /etc/sutra/model-manifest.json

RUN chmod 0755 /usr/local/bin/fetch-model

ENTRYPOINT ["/usr/local/bin/fetch-model"]
