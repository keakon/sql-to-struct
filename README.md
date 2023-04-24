# 用法

```bash
go run . <file_path> [<table_name>] [mode=sql/json/sb/sql+sb/...]
```

## 生成所有表

```bash
go run . ~/Workspace/db/sql/mysql/createdb.sql  # 改为实际路径
```

## 生成指定表

```bash
go run . createdb.sql user
```

## 设置输出模式

```bash
go run . createdb.sql mode=sql+sb
```

`mode` 可以为 `sql`、`json` 和 `sb`，以及它们用 `+` 连接的组合，默认为 `json+sb`。

### 示例输出

* sql
	```go
	type User struct {
		ID       uint32 `db:"id"`
		TenantID uint16 `db:"tenant_id" json:"tenant_id"`
	}
	```
* json
	```go
	type User struct {
		ID       uint32 `db:"id" json:"id"`
		TenantID uint16 `db:"tenant_id" json:"tenant_id"`
	}
	```
* sb
	```go
	type UserTable struct {
		sb.Table `db:"user"`
		ID       sb.Column `db:"id"`
		TenantID sb.Column `db:"tenant_id"`
	}
	```
