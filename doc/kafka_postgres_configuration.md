# Configuring Kafka/Postgres for a Production Environment

## Configuring Kafka

### Preparing the Environment

  - To achieve more efficient I/O performance and fast data transfer, it is recommended to deploy Kafka on Linux systems

  - Kafka needs open file descriptors for files and network connections. You should set the file descriptor limit to at least `128000`

  - It's recommended using the `EXT4` or `XFS` file system

  - Use the latest update for Java version 1.8 and make sure that G1 garbage collection support is enabled.
    Here are several recommended settings for the JVM:
    ```
    -Xmx6g
    -Xms6g
    -XX:MetaspaceSize=96m
    -XX:+UseG1GC
    -XX:MaxGCPauseMillis=20
    -XX:InitiatingHeapOccupancyPercent=35
    -XX:G1HeapRegionSize=16M
    -XX:MinMetaspaceFreeRatio=50
    -XX:MaxMetaspaceFreeRatio=80
    ```

  - Ethernet bandwidth can have an impact on Kafka performance; make sure it is sufficient for your throughput requirements.

### Kafka Broker Setting

#### Topic setting

- Partition - `num.partitions`, the minimum number of required partition is `max (T/P, T/C)`. 

  - `P`, throughput from a producer to a single partition
  - `C`, throughput from a single partition to a consumer
  - `T`, target throughput

  Now suppose we have `3` managed hub clusters, each with `250` managed clusters, and each cluster has `10` policies. The status of all these policies need to be updated within `10` minutes. Then up to `7500` messages can be sent to the manager. Assuming that the throughput of each partition is `500/min`, then the maximum number of partitions required is `7500/(500*10)=1.5`. Then in order to achieve the above performance, you need to set up `2` partitions for each topic.

- Replication - `default.replication.factor`

  For high availability production systems, you should set this value to at least 3.

#### Log setting

- `log.dirs` 

  A comma-separated list of directories in which log data is kept. If you have multiple disks, list all directories under each disk.
  
- Data Retention - `log.retention.hours`

  The higher the retention setting, the longer the data will be preserved. Higher settings generate larger log files, so increasing this setting might reduce your overall storage capacity. Because the global hub mainly depends on postgres storage, it's recommended to set `48` hours.

#### General Broker Settings

- `unclean.leader.election.enable` - recommend to `false`

- `auto.leader.rebalance.enable` - recommend to `false`

- Kafka Broker Size

  Kafka brokers require medium sized instances. The size of the broker is directly dependent on size of the data and rate of the data flow. Adding new brokers to the cluster helps in the way that it will spread the partitions across the brokers and reduce the stress over the brokers. It is good to start with at least `4 cores` and `16 GB` of RAM.

- Kafka Broker Number

  We have set the replication-factor for production is 3, so we need at least `3 brokers` in order to achieve the Kafka HA Cluster.

#### Configuring ZooKeeper for Use with Kafka

Here are several recommendations for ZooKeeper configuration with Kafka:

- Do not run ZooKeeper on a server where Kafka is running
- When using ZooKeeper with Kafka you should dedicate ZooKeeper to Kafka, and not use ZooKeeper for any other components.
- Make sure you allocate sufficient JVM memory. A good starting point is `4GB`.
- To monitor the ZooKeeper instance, use JMX metrics.
- Advised to use at least `3 nodes` in the ZooKeeper ensemble.

## Configuring Postgres

### Backups

Backups in the database world are essential. They are the safety net protecting you from even the smallest bit of data loss.

- `pg_dump/pg_restore`

  Using the basic tool `pg_dump/pg_restore` to backup and restore data, you can visit [here](./troubleshooting.md#database-dump-and-restore) for more detail. 

- Amazon Relational Database Service Backups
  
  If you choose to provision the postgres on AWS RDS, You can enable this feature by referring to the [relevant documentation](https://aws.amazon.com/rds/features/backup). 
  
- [PGO](https://github.com/CrunchyData/postgres-operator) Backups

  An important part of a healthy Postgres cluster is maintaining backups. PGO optimizes its use of open source `pgBackRest` to be able to support terabyte size databases. Please refer to this [document](https://github.com/CrunchyData/postgres-operator/blob/master/docs/content/tutorial/backups.md) for details

### Highly Available

In the case of database high availability, your core data should exist in at least two separate environments, all of which are up to date, with at least one of them ready to switch to the primary node if your main database fails. The technical specifics and implementation of HA can often be complex. You can boil HA Postgres down into the following key topics:

- Replication: a main database with one or more replicas

  A tool for keeping external IPs the same so you don’t have to update your app code if you fail over to a new database server. This could be an elastic IP or a proxy, or other DNS configurations.

- Monitoring: a monitor for each instance and dashboard and alert system for each instance

- Backups

Amazon Relational Database Service (Amazon RDS) supports using [Multi-AZ deployments](https://aws.amazon.com/rds/ha/) to ensure High Availability. [PGO](https://github.com/CrunchyData/postgres-operator) also expose the [Replicas and Affinity]((https://github.com/CrunchyData/postgres-operator/blob/master/docs/content/tutorial/high-availability.md)) to provide a HA Postgres cluster.

### Logging

Postgres logs are a valuable resource for troubleshooting problems, tracking performance, and auditing database activity. Here are some recommended best practices.

- Auto log [slow queries](https://www.crunchydata.com/blog/logging-tips-for-postgres-featuring-your-slow-queries)

- Auto the [explain plan](https://docs.crunchybridge.com/extensions-and-languages/auto_explain/?CrunchyAnonId=mimiduhdafawqsoqiolyrsfqbggntoiiiuaqlzacofvechs) of slow queries in log

### Connection Pool

Connection pooling can be helpful for scaling and maintaining overall availability between the global hub and the database. 

- [Enable connection pooling on PGO](https://github.com/CrunchyData/postgres-operator/blob/master/docs/content/tutorial/connection-pooling.md)

- [Guide to Connection Management in Postgres](https://www.crunchydata.com/blog/your-guide-to-connection-management-in-postgres)

### Others

- [Control Runaway Postgres Queries With Statement Timeout](https://www.crunchydata.com/blog/control-runaway-postgres-queries-with-statement-timeout)

- [Optimize Postgres Server Performance Through Configuration](https://www.crunchydata.com/blog/optimize-postgresql-server-performance)

## Reference

- [Installing and configuring Apache Kafka](https://docs.cloudera.com/HDPDocuments/HDP3/HDP-3.1.5/installing-configuring-kafka/content/configuring_kafka_for_a_production_environment.html)

- [Running Kafka in Production](https://docs.confluent.io/platform/current/kafka/deployment.html)

- [Introduction to Postgres Backups](https://www.crunchydata.com/blog/introduction-to-postgres-backups)

- [Postgres High Availability and Disaster Recovery](https://www.crunchydata.com/blog/database-terminology-explained-postgres-high-availability-and-disaster-recovery)

- [Why PostgreSQL High Availability Matters and How to Achieve It](https://www.yugabyte.com/postgresql/postgresql-high-availability/)