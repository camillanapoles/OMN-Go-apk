docker buildx build -f local/Dockerfile.compare --progress=plain --build-arg RUN_ID=$(date +%s) --build-arg SIZE_BASE=${1} --output type=cacheonly .
