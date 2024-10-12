# Multicluster Global Hub Agent

The **Global Hub Agent** component is responsible for applying resources to the hub cluster (spec path) and reporting the resource status to the Global Hub Manager via Kafka. It also synchronizes events with the Inventory API (status path). Additionally, the agent can be run in [standalone mode](./../doc/event-exporter/README.md), which only enables the status path feature.

## Structure

- **agent**
  - **cmd**: Command-line utilities for the agent.
  - **pkg**: Contains the core logic and functionalities.
    - **configs**: Holds configurations, schemas, and related assets.
    - **controllers**: Common controllers, including initialization, cluster claim, and lease controllers.
      - **inventory**: The controllers that report resources via the inventory API.
    - **spec**:
      - **rbac**: Manages role-based access control.
      - **syncers**: Syncs resources and signals from the Global Hub Manager.
      - **workers**: Backend goroutines that execute tasks received from the spec syncers.
    - **status**:
      - **filter**: Deduplicates events when reporting resource statuses.
      - **generic**: Common implementations for the status syncer.
        - **controller**: Specifies the types of resources to be synced.
        - **handler**: Updates the bundle synced to the manager by the watched resources.
        - **emitter**: Sends the bundle created/updated by the handler to the transport layer (e.g., via CloudEvents).
        - **multi-event syncer**: A template for sending multiple events related to a single object, such as the policy syncer.
        - **multi-object syncer**: A template for sending one event related to multiple objects, such as the managedhub info syncer.
      - **interfaces**: Defines the behaviors for the Controller, Handler, and Emitter.
      - **syncers**: Specifies the resources to be synced, following templates provided by the generic syncers.