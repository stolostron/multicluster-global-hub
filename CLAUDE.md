# Hub-of-Hubs Project Knowledge Base

> 本文档记录了Hub-of-Hubs项目中Migration系统的完整设计和实现知识

## Migration系统架构与流程知识

### 概述
本项目实现了一个完整的集群迁移系统，支持将managed clusters从一个hub迁移到另一个hub。系统采用事件驱动架构，通过Global Hub作为协调中心来管理整个迁移流程。

### 核心组件

#### 1. Migration Controller
- 位置：`manager/pkg/migration/migration_controller.go`
- 职责：协调整个迁移流程，管理各个阶段的状态转换
- 主要方法：`Reconcile()` - 主要的调度循环

#### 2. 阶段处理器
每个迁移阶段都有对应的处理文件：
- `migration_validating.go` - 验证阶段
- `migration_initializing.go` - 初始化阶段  
- `migration_deploying.go` - 部署阶段
- `migration_registering.go` - 注册阶段
- `migration_rollbacking.go` - 回滚阶段 (新增)
- `migration_cleaning.go` - 清理阶段

#### 3. 类型定义
- 位置：`operator/api/migration/v1alpha1/managedclustermigration_types.go`
- 定义了所有的Phase和Condition类型

### 迁移流程设计

#### 正常流程
```
Pending → Validating → Initializing → Deploying → Registering → Cleaning → Completed
```

#### 失败处理流程
```
- Validating (失败) → Failed
- Initializing/Deploying/Registering (失败) → Rollbacking → Failed  
- Rollbacking (成功/失败) → Failed
- Cleaning (成功/失败) → Completed (失败时带warning)
```

### 各阶段详细说明

#### 1. Validating阶段
- **目的**：验证源Hub、目标Hub和集群的有效性
- **失败处理**：直接进入Failed状态
- **关键验证**：
  - Hub集群存在性验证
  - 目标集群名称冲突检查
  - 源集群存在性验证

#### 2. Initializing阶段  
- **目的**：准备迁移环境
- **操作**：
  - 创建ManagedServiceAccount
  - 生成bootstrap secret
  - 向源Hub和目标Hub发送初始化事件
- **失败处理**：进入Rollbacking阶段

#### 3. Deploying阶段
- **目的**：将资源从源Hub迁移到目标Hub
- **操作**：
  - 源Hub准备资源
  - 目标Hub接收并应用资源
- **失败处理**：进入Rollbacking阶段

#### 4. Registering阶段
- **目的**：将集群重新注册到目标Hub
- **操作**：
  - 源Hub更新集群配置
  - 目标Hub接收集群注册
- **失败处理**：进入Rollbacking阶段

#### 5. Rollbacking阶段 (新增)
- **目的**：在迁移失败时恢复系统到原始状态
- **操作**：
  - 向源Hub发送回滚事件以恢复原始配置
  - 向目标Hub发送回滚事件以清理部分资源
  - 清理ManagedServiceAccount
  - **清理managed cluster annotations** (重要):
    - 移除 `constants.ManagedClusterMigrating` annotation
    - 移除 `KlusterletConfigAnnotation` annotation
    - 通过发送"AnnotationCleanup"事件给源Hub的agent处理
- **结果**：无论成功失败都进入Failed状态
- **设计理念**：回滚是恢复性操作，一旦需要回滚就表明迁移失败

#### 6. Cleaning阶段
- **目的**：清理迁移过程中的临时资源
- **操作**：
  - 删除ManagedServiceAccount
  - 清理源Hub和目标Hub的迁移相关资源
- **结果**：无论成功失败都进入Completed状态
- **设计理念**：清理失败不影响迁移成功，只需要warning提醒手动清理

### 错误处理策略

#### 1. 不同阶段的错误处理
- **Validating**：快速失败，直接进入Failed
- **Initializing/Deploying/Registering**：尝试回滚，然后Failed
- **Rollbacking**：无论结果都进入Failed  
- **Cleaning**：无论结果都进入Completed（失败时带warning）

#### 2. 状态处理函数
- `handleMigrationStatus()` - 通用状态处理，用于大部分阶段
- `handleRollbackStatus()` - 回滚专用状态处理
- `handleCleaningStatus()` - 清理专用状态处理

### 事件驱动架构

#### 1. 事件类型
- `MigrationSourceMsgKey` - 发往源Hub的事件
- `MigrationTargetMsgKey` - 发往目标Hub的事件

#### 2. 特殊事件类型
- `PhaseInitializing` - 初始化事件，会在managed cluster上添加migration annotations
- `PhaseDeploying` - 部署事件，处理资源迁移
- `PhaseRegistering` - 注册事件，重新注册集群
- `PhaseCleaning` - 清理事件，移除临时资源和annotations
- `PhaseRollbacking` - **回滚事件**，包含RollbackStage字段指示回滚哪个阶段

#### 3. 状态同步机制
- `AddMigrationStatus()` - 添加迁移状态跟踪
- `GetStarted()` / `SetStarted()` - 跟踪阶段启动状态
- `GetFinished()` / `SetFinished()` - 跟踪阶段完成状态
- `GetErrorMessage()` - 获取错误信息

### Migration Annotations管理

#### 1. 关键Annotations
- **`constants.ManagedClusterMigrating`**: 
  - 值：`"global-hub.open-cluster-management.io/migrating"`
  - 作用：标记集群正在迁移中，Global Hub agent会忽略该集群的状态报告
  - 添加时机：initializing阶段
  - 清理时机：cleaning阶段或rollback时
  
- **`KlusterletConfigAnnotation`**:
  - 值：`"agent.open-cluster-management.io/klusterlet-config"`
  - 作用：指向用于迁移的KlusterletConfig资源名称
  - 添加时机：initializing阶段
  - 清理时机：cleaning阶段或rollback时

#### 2. Annotation生命周期
```
Initializing阶段：
└── 源Hub agent添加annotations到managed clusters

正常清理流程：
└── Cleaning阶段：源Hub agent移除annotations

失败回滚流程：
└── Rollbacking阶段：发送PhaseRollbacking事件（包含RollbackStage字段）
    ├── 源Hub agent根据RollbackStage执行相应回滚操作
    │   ├── PhaseInitializing: 移除annotations
    │   ├── PhaseDeploying: 移除annotations
    │   └── PhaseRegistering: 移除annotations + 恢复注册配置
    └── 目标Hub agent根据RollbackStage执行相应回滚操作
        ├── PhaseInitializing: 清理RBAC资源
        ├── PhaseDeploying: 删除ManagedCluster + KlusterletAddonConfig + RBAC
        └── PhaseRegistering: 删除ManagedCluster + KlusterletAddonConfig + RBAC
```

#### 3. Agent端处理逻辑

**源Hub Agent** (`agent/pkg/spec/syncers/migration_from_syncer.go`):
- `initializing()` - 添加annotations
- `cleaning()` - 移除annotations（正常流程）
- `rollbacking()` - 处理回滚操作，根据RollbackStage分发
- `rollbackInitializing()` - 回滚初始化阶段，移除annotations
- `rollbackDeploying()` - 回滚部署阶段，移除annotations
- `rollbackRegistering()` - 回滚注册阶段，移除annotations + 恢复注册配置

**目标Hub Agent** (`agent/pkg/spec/syncers/migration_to_syncer.go`):
- `initializing()` - 创建RBAC资源
- `deploying()` - 接收并应用ManagedCluster和KlusterletAddonConfig
- `cleaning()` - 清理资源（正常流程）
- `rollbacking()` - 处理回滚操作，根据RollbackStage分发
- `rollbackInitializing()` - 回滚初始化阶段，清理RBAC资源
- `rollbackDeploying()` - **重点**：删除ManagedCluster + KlusterletAddonConfig + RBAC
- `rollbackRegistering()` - 回滚注册阶段，执行与部署回滚相同的清理

#### 4. Event结构改进
- **ManagedClusterMigrationFromEvent**新增字段：
  - `RollbackStage string` - 指示正在回滚哪个阶段
  - 当Stage为PhaseRollbacking时，RollbackStage指明具体回滚操作

- **ManagedClusterMigrationToEvent**新增字段：
  - `RollbackStage string` - 指示正在回滚哪个阶段
  - 用于目标Hub的回滚操作

#### 5. Deploying阶段回滚的关键操作
当deploying阶段失败时，rollback操作包括：

**源Hub端**：
1. 移除managed cluster上的migration annotations
   - `constants.ManagedClusterMigrating`
   - `KlusterletConfigAnnotation`

**目标Hub端**：
1. 删除已创建的ManagedCluster资源
2. 删除已创建的KlusterletAddonConfig资源  
3. 清理migration相关的RBAC资源（ClusterRole和ClusterRoleBinding）

这确保了deploying失败后，target hub上不会遗留任何部分创建的资源。

### 关键设计决策

#### 1. 为什么Rollback总是进入Failed？
- 回滚表明原始迁移已经失败
- 回滚是恢复性操作，不是正常流程的一部分
- 即使回滚成功，整个迁移仍应视为失败

#### 2. 为什么Cleaning失败仍进入Completed？
- 核心迁移功能已完成（集群已成功迁移）
- 清理失败只影响资源回收，不应影响迁移成功状态
- 通过warning提醒管理员手动清理剩余资源

#### 3. 超时处理
- 不同阶段有不同的超时时间
- `migrationStageTimeout = 5分钟` - 大部分阶段
- `registeringTimeout = 12分钟` - 注册阶段（需要更长时间）
- `CleaningTimeout = 10分钟` - 清理阶段

### 最佳实践

#### 1. 错误处理
- 每个阶段都应该有明确的错误处理策略
- 使用defer确保状态更新的一致性
- 错误信息应该包含足够的上下文信息

#### 2. 状态管理
- 使用condition来跟踪详细状态
- phase表示当前主要阶段
- 通过message提供用户友好的状态信息

#### 3. 事件驱动
- 异步处理，避免长时间阻塞
- 使用状态机模式管理复杂的状态转换
- 确保事件的幂等性

### 常见问题排查

#### 1. 迁移卡在某个阶段
- 检查对应Hub的agent状态
- 查看事件消息是否正确发送
- 检查网络连接和权限

#### 2. 回滚失败
- 检查源Hub和目标Hub的连接状态
- 确认ManagedServiceAccount是否存在
- 查看具体的错误消息

#### 3. 清理失败但仍显示Completed
- 这是正常行为，检查condition中的warning信息
- 根据warning信息进行手动清理
- 关注ManagedServiceAccount等资源的状态

### 未来改进方向

1. **增强监控**：添加更多的metrics和监控指标
2. **并发支持**：支持多个迁移同时进行
3. **增量迁移**：支持部分资源的增量迁移
4. **迁移验证**：迁移后的自动验证机制
5. **性能优化**：大规模集群迁移的性能优化