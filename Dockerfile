# Simple two-container builder optimized for Go.
# One container (golang:alpine) includes all of the Go toolchain.
# We build code there, then move the binary into an alpine container to run.
# This provides the smallest container size.
# And, since Go makes binaries, we don't need any toolchain in the final container.

FROM golang:alpine as build
COPY . /app
WORKDIR /app
# Flags reduce binary size.
# -w removes DWARF debugging information (for gdb)
# -s removes Go's symbol table so you can't list functions.
# Neither of these affect how the program runs, just how it can be debugged.
# More info: https://stackoverflow.com/a/22276273
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/compiled-binary *.go

# Make sure to always use the same version of linux for building AND running!
FROM alpine
# Move the compiled binary file from the build container to the run container.
COPY --from=build /app/bin/compiled-binary /app-binary
# This environment variable is evaluated at runtime, not compiletime.
EXPOSE $PORT
ENTRYPOINT ["/app-binary"]
