# STAGE 2: The quality gate, then the artifacts.
#
# The test stage and the project_builder stage are the same as in
# Dockerfile.ci. Keep the two files in step. project_builder is the last
# stage, thus local/build.sh builds it. Run the gate alone with:
#   docker buildx build --target test .

# ------------------------------ test ---------------------------------
# The quality gate: go vet and the unit tests. The JDK and Node of the
# base image also run the Java test and the JavaScript tests. The last
# step writes /gate-passed. project_builder copies that file, thus no
# artifact comes from a build with a failed gate. The gate runs WITHOUT
# the release GOFLAGS (-s -w -trimpath) of the desktop build step.
FROM omn-go-base:latest AS test

# Set to 1 to skip the test gate for an emergency build:
#   docker build --build-arg SKIP_TESTS=1 ...
ARG SKIP_TESTS=0

COPY . .

# Restore the full go.mod and go.sum from /root/lockfiles. The "COPY . ."
# above brought in the files of the host.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# A check, and not a resolution step. go mod tidy compares go.mod with
# the source. It changes nothing and uses no network while go.mod agrees
# with the imports of the source.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    if [ "$SKIP_TESTS" = "1" ]; then \
        echo "WARNING: SKIP_TESTS=1 - test gate bypassed"; \
    else \
        go vet ./backend/... && \
        go test ./backend/...; \
    fi && \
    touch /gate-passed

# -------------------------- project_builder --------------------------
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

# This copy makes the test stage a part of each build of this stage.
COPY --from=test /gate-passed /tmp/gate-passed

COPY . .

# Restore the full go.mod and go.sum from /root/lockfiles. The "COPY . ."
# above brought in the files of the host.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# A check, and not a resolution step. go mod tidy compares go.mod with
# the source. It changes nothing and uses no network while go.mod agrees
# with the imports of the source.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

# Desktop Binary (OMN-Go naming convention)
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-ldflags=-s -w -trimpath" && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
#
# -trimpath is a flag of gomobile, and not of the linker. gomobile writes
# the generated Go code into a new temporary directory for each run.
# Without -trimpath, libgojni.so holds that path, thus two builds of one
# commit differ.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    mkdir -p android/app/libs && \
    gomobile bind -trimpath -target=android -androidapi 23 -javapkg net.basov.omngo -ldflags="-s -w" -o android/app/libs/omngo.aar ./backend

# Explicitly the "standard" flavor only - never "fdroid". The fdroid
# variant is built exclusively by F-Droid's own build server, from its
# own recipe (see metadata/net.basov.omngo.fdroid.yml); building it here
# would be pointless (F-Droid re-signs with its own key regardless) and
# risks producing/publishing an APK that looks official but isn't what
# F-Droid actually distributes.
RUN --mount=type=cache,target=/root/.gradle,sharing=locked \
    cd android && \
    gradle assembleStandardRelease && \
    cp app/build/outputs/apk/standard/release/*.apk ../bin/
