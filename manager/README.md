# Multicluster Global Hub Manager

The Global Hub Manager is responsible for distributing workloads across clusters and collecting resource status data into its database via transport.

## Structure

- **manager**
  - **cmd**: Command-line utilities for the manager.
  - **pkg**: Core logic and functionalities.
    - **configs**: Holds configurations, schemas, and related assets.
    - **controllers**: Includes common controllers such as migration and backup controllers.
    - **processes**: Periodically running internal jobs, such as the policy compliance cronjob and managed hub cluster management job.
    - **restapis**: Exposes REST APIs, such as managed clusters, policies, and subscriptions.
    - **spec**:
      - **specdb**: Handles database operations for synchronizing resources to the database and retrieving them for transport.
      - **controllers**: Watches resources and persists them in the database.
      - **syncers**: Syncs resources from the database and sends them via transport.
    - **status**:
      - **conflator**: Merges bundles inserted by transports and prepares them for dispatch.
      - **dispactcher**: Routes bundles or events between components, from transport to conflator, and delivers bundles from the conflator to the database worker pool.
      - **handlers**: Defines how transferred bundles are persisted in the database.
    - **webhook**: The webhooks, such as disabling placement controllers for the global resource.