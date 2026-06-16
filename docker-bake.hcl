group "default" {
    targets = ["binary"]
}

target "dev" {
    target  = "build"
    tags    = ["opendivine-dev"]
}

target "binary" {
    dockerfile = "Dockerfile"
    context = "."
    target = "binary"
    output = ["type=local,dest=_build"]
    attest = [
        { type = "provenance", mode = "max" },
        { type = "sbom" }
    ]
    platforms = ["local"]
}
