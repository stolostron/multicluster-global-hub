# Deployment instructions for Hub-of-Hubs

1.  Define the `KUBECONFIG` variable to hold the kubernetes configuration of Hub-of-Hubs

1.  Define the REGISTRY environment variable, for example:

    ```
    export REGISTRY=docker.io/<your user name>
    ```

1.  Define the release tag variable for images:

    ```
    export IMAGE_TAG=v0.1.0
    ```

1.  Define the `DATABASE_URL_HOH` and `DATABASE_URL_TRANSPORT` variables for the HoH and Transport users

Set the `DATABASE_URL` according to the PostgreSQL URL format: `postgres://YourUserName:YourURLEscapedPassword@YourHostname:5432/YourDatabaseName?sslmode=verify-full`.

:exclamation: Remember to URL-escape the password, you can do it in bash:

```
python -c "import sys, urllib as ul; print ul.quote_plus(sys.argv[1])" 'YourPassword'
```

1.  Define `SYNC_SERVICE_HOST` environment variable

1.  Run:

    ./deploy_hub_of_hubs.sh

## Linting

**Prerequisite**: install the `shellcheck` tool (a Linter for shell):

```
brew install shellcheck
```

Run
```
shellcheck *.sh
```
