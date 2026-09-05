# AWS Deployment Notes

Target: get job-find running on AWS within the existing $100 credit
(README §5/§6). This is a pragmatic single-region layout, not a
multi-AZ production hardening guide.

## Minimal footprint (fits the credit)

| Component   | AWS service                              | Notes                                   |
|-------------|------------------------------------------|-----------------------------------------|
| Postgres    | RDS for PostgreSQL + PostGIS, `t3.micro` | Free-tier eligible; enable PostGIS ext. |
| Redis       | ElastiCache `t3.micro`, or Redis on EC2  | Asynq broker.                           |
| API         | ECS Fargate (1 task) or a `t3.small` EC2 | Runs `Dockerfile.api`.                   |
| Ingestion   | ECS Fargate (1 task) or same EC2         | Runs `Dockerfile.ingest` (`worker`).    |
| Frontend    | ECS Fargate, Amplify, or Vercel          | Runs `Dockerfile.frontend`.             |
| TLS + DNS   | ACM cert + Route 53 for `job-find.xyz`   | ALB or CloudFront in front.             |

For the very cheapest start, run API + ingest + frontend as three containers on
one `t3.small` via `docker compose`, with RDS + ElastiCache as managed stores.

## PostGIS on RDS

RDS Postgres supports PostGIS but the extension must be created once:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
```

The app's migration runner (`0001_init_schema.sql`) issues this, so as long as
the DB user has permission it happens automatically on first boot.

## Secrets

Inject these as task/environment secrets (never bake into an image):
`DATABASE_URL`, `REDIS_ADDR`, `JWT_SECRET`, and any of `ADZUNA_APP_ID` /
`ADZUNA_APP_KEY` / `JOOBLE_API_KEY` / `GOOGLE_MAPS_API_KEY` you use. Use AWS
Secrets Manager or SSM Parameter Store.

## Budget alerts (do this first)

1. **AWS Budgets** — alert at, say, $50 and $90 of the $100 credit.
2. **Google Cloud budget** — only if you enable a Google Maps/Geocoding key;
   tripwires at $50 / $150 / $190 (README §6).

## Scaling later

- Put the API behind an ALB and raise the Fargate task count.
- Move the response cache from in-process to ElastiCache so it's shared across
  API tasks.
- Add a CloudFront distribution in front of the frontend and the tiles.
