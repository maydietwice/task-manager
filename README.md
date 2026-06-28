# GRPC Task Manager

Microservice for managing your tasks via gRPC api

## Actual stack:

- **Language:** Go v1.26
- **Communication:** gRPC (Protocol buffers)
- **Database:** PostgreSQL v18
- **Security auth:** [JWT Tokens](https://github.com/golang-jwt/jwt)
- **Infrastructure:** Docker & Docker Compose

## Configuration

Create .env configuration (see `env.example`):  

| Variable | Description | Example |
|---|---|---|
| NET_LISTEN_ADDRESS | gRPC server port | :50051 |
| DB_CONNECTION_STRING | [PostgreSQL connection string](https://github.com/lib/pq) | host=postgres port=5432 user=user password=password dbname=dbname sslmode=disable
| POSTGRES_DB | Database name | dbname |
| POSTGRES_USER | Database user | user |
| POSTGRES_PASSWORD | Database password | password |
| JWT_SECRET_KEY | JWT secret key | banana |
| DB_MAX_OPEN_CONNS | Maximum number of simultaneous database connections | 15 |
| DB_MAX_IDLE_CONNS | Maximum number of idle connections in pool | 5 |
| DB_CONN_MAX_IDLE_TIME | Maximum time for keeping alive and idle connection | 15m |
| DB_CONN_MAX_LIFETIME | Maximum time for keeping alive an active connection | 1h |

## Quick start

**Requirements**
- Docker & Docker Compose: [Docker official documentation](https://docs.docker.com/get-docker/)

**Launching project**
1. Clone repository:  
```bash
git clone github.com/maydietwice/grpc-taskmanager
```
2. Go to directory:  
```bash
cd grpc-taskmanager
```
3. Create .env configuration (see `env.example`):
4. Launch project:  
```bash
docker compose up --build
```

**Useful commands**
1. Stop project:  
```bash
docker compose down
```
2. Check logs:  
```bash
docker compose logs -f
```  

## gRPC API ([Protocol Buffers](https://protobuf.dev/))

### Task structure  
```json
{
    "id":"string",
    "owner_id":"string",
    "title":"string",
    "status":"Status",
    "description":"string",
    "created_at":"google.protobuf.Timestamp",
    "updated_at":"google.protobuf.Timestamp"
}
```
### Status enum  
```json
{
    "STATUS_PENDING":0,
    "STATUS_INPROGRESS":1,
    "STATUS_DONE":2
}
```
### Register()  

Returns your personal `token` that you should save. Used to relate your task to your connection

**Request**  
`Request structure is not required`

**Response**  
```json
{
    "token":"string"
}
```

### CreateTask()  
Requests `title` and `description` of a new task, saves it in database and returns it

**Request**
```json
{
    "title":"string",
    "description":"string"
}
```

**Response**
```json
{
    "task":"Task"
}
```

### DeleteTask()  
Deletes task from database by `id` 

**Request**
```json
{
    "id":"string"
}
```

**Response**  
```json
{
    "success":"boolean"
}
```

### GetTask()  
Gets task from database by `id` and returns it  

**Request**
```json
{
    "id":"string"
}
```

**Response**  
```json
{
    "task":"Task"
}
```

### UpdateTask()
`Necessary requests: id`  
`Soft requests(leave field empty if not needed to update): status, title, description`  

Updates task in database using soft requests fields and returns it 

**Request**
```json
{
    "id":"string",
    "status":"Status",
    "title":"string",
    "description":"string"
}
```

**Response**  
```json
{
    "task":"Task"
}
```

### ListTask()
Returns an array of tasks from `page` in amount of `limit`  
Page is being calculated by (`page` - 1) * `limit`  
Tasks are sorted by `created_at`

**Request**
```json
{
    "page":"int",
    "limit":"int"
}
```

**Response**
```json
{
    "tasks": [
        {
            "task":"Task"
        }
    ]
}
```
 