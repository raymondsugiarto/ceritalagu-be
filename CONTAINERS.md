# Containers and deployment

## Local development

The backend Compose file starts PostgreSQL, Redis, the API, and the generation
worker on the shared `music-dev` network. The frontend has its own Compose file
and joins that network to proxy `/api/*` directly to the API container.

```sh
cp -n .env.example .env
```

Set `SECURITY_MASTERKEY` in `.env` to the output of `openssl rand -base64 32`,
then run:

```sh
docker compose up --build -d
cd ../frontend
docker compose up --build -d
```

The site is at `http://localhost:3001`; API health is at
`http://localhost:8080/healthz`. Local audio, PostgreSQL, and Redis data use
named Docker volumes. The local database password defaults to `postgres`;
change it if the database port is reachable beyond your machine. Stop a
host-run API first if it already listens on port 8080; the existing
localhost.run forward to that port will then reach the container API.

To enable DOKU sandbox checkout, set `PAYMENT_DOKU_CLIENTID` and
`PAYMENT_DOKU_SECRETKEY` in the backend `.env`, use
`PAYMENT_DOKU_BASEURL=https://api-sandbox.doku.com`, and set
`PAYMENT_PUBLICBASEURL=http://localhost:3001`. For an internet callback,
forward port 8080 with ngrok or localhost.run and register the public HTTPS
URL plus `/api/v1/payments/doku/notifications` in DOKU. Keep
`PAYMENT_DOKU_NOTIFYPATH` set to that path.

## Portainer production stacks

Create the Docker network named `platform` on the Portainer endpoint. Deploy
the backend stack using `docker-compose.portainer.yml` first, then the
frontend stack using the file with the same name in `../frontend`. Both
services join `platform`; the frontend calls the backend at
`http://music-api:8080`. The frontend host port remains `6020`.

Set these variables in the backend Portainer stack:

- Required: `DATABASE_MAIN_PASSWORD`, `REDIS_PASSWORD`, `SECURITY_MASTERKEY`,
  `SECURITY_IPHASHSALT`, `MAIL_BREVO_APIKEY`, `MAIL_BREVO_SENDEREMAIL`
  (a verified Brevo sender), and `PAYMENT_PUBLICBASEURL`.
- Email goes through Brevo (`MAIL_DRIVER` defaults to `brevo`); optionally set
  `MAIL_BREVO_SENDERNAME`.
- Optional: `IMAGE_OWNER` (defaults to `raymondsugiarto`), `DATABASE_MAIN_USER`,
  `STORAGE_DRIVER` and `STORAGE_S3_*`, `NOTIFICATION_*`, and DOKU settings
  `PAYMENT_DOKU_CLIENTID` / `PAYMENT_DOKU_SECRETKEY`. DOKU payments remain off
  until both credentials are configured. Use the sandbox base URL for testing;
  production defaults to `https://api.doku.com`.

The GitHub Actions publish workflows push to GHCR using `GITHUB_TOKEN` and
redeploy the matching Portainer stack on `main` or `master`. Configure these
repository Actions secrets in each app repository:
`PORTAINER_URL`, `PORTAINER_USERNAME`, `PORTAINER_PASSWORD`,
`PORTAINER_STACK_ID`, and `PORTAINER_ENDPOINT_ID`. The workflow preserves the
existing stack environment when it updates the Compose definition.
If the GHCR packages are private, configure the Portainer registry credentials
for the same GitHub account or deploy token.

Do not put credentials in the image or commit them. In particular, keep
per-machine `config/resources/` out of Docker build contexts; the image uses
the safe templates under `config/example/`.
