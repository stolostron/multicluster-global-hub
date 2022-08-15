package system.authz

# Deny access by default.
default allow = false


# Allow GET only
allow {
    input.method = "GET"
}

# Allow POST to /v1/compile - partial evaluations
allow {
    input.method = "POST"
    input.path[0] = "v1"
    input.path[1] = "compile"
}

# Allow POST to /v1/data/rbac/clusters/allow
allow {
    input.method = "POST"
    input.path[0] = "v1"
    input.path[1] = "data"
    input.path[2] = "rbac"
    input.path[3] = "clusters"
    input.path[4] = "allow"
}