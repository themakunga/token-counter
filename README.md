# token-counter

Terminal TUI que muestra el uso de tokens y costo mensual estimado de tus agentes de IA en tiempo
real.

```
╭────────────────────────────────────────────────────────────────────────────────────────────────────╮
│  Agent             Hourly                    Daily                    Weekly           Monthly       ⬤ │
│  ──────────────────────────────────────────────────────────────────────────────────────────────   │
│  Claude Code       1.2K / 100K               45.6K / 1M              234K / 5M        2.1M / 20M  ● │
│  OpenAI / Codex    2.3K / 50K                89K / 500K              1.2M / 2M        5.4M / 8M   ● │
│                                                                                                    │
│  ─────────────────────────────────────────────────────────────────────────────────────────────   │
│  Monthly spend                                                                                     │
│    Claude Code     $4.23  / $50.00 budget  ($45.77 avail)                                        │
│    OpenAI / Codex  $1.20                                                                          │
│                                                                                                    │
│  last: 10:30:45  •  r=refresh  q=quit                                                             │
╰────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

**Colores:** 🟢 < 50% del límite · 🟡 50–80% · 🔴 > 80% **Monthly spend:** colorea según porcentaje
del presupuesto configurado.

---

## Descarga

### Binarios pre-compilados (recomendado)

| Canal       | Cuándo usar                        | Link                                    |
| ----------- | ---------------------------------- | --------------------------------------- |
| **Estable** | Producción / uso diario            | [Última versión](../../releases/latest) |
| **Nightly** | Probar cambios recientes de `main` | [nightly](../../releases/tag/nightly)   |

Descarga el binario para tu plataforma y muévelo al PATH:

```bash
# macOS Apple Silicon (ejemplo v1.0.0)
curl -L https://github.com/TU-USUARIO/token-counter/releases/download/v1.0.0/token-counter-darwin-arm64 \
  -o /usr/local/bin/token-counter
chmod +x /usr/local/bin/token-counter

# macOS Intel
curl -L .../token-counter-darwin-amd64 -o /usr/local/bin/token-counter && chmod +x $_

# Linux amd64
curl -L .../token-counter-linux-amd64 -o /usr/local/bin/token-counter && chmod +x $_
```

Verificar checksum (recomendado):

```bash
sha256sum -c checksums.txt
```

### Modelo de ramas (Trunk-Based Development)

```
main            ──────●──────●──────●──────  → nightly (inestable, cada push)
                            ╲
release/v1.0.0               ●──────────────  → tag v1.0.0 (estable)
release/v1.1.0                        ●──────  → tag v1.1.0 (estable)
```

- **`main`** → compilación nightly automática en cada push
- **`release/vX.Y.Z`** → crea el tag `vX.Y.Z` y publica la release estable
- El pipeline valida Go y Nix antes de publicar. Escribe la versión en `VERSION`, la incluye en el
  commit etiquetado y comprueba que el binario Nix responde `vX.Y.Z` a `--version`. Solo después de
  publicar avanza el tag `stable`.
- Los tags `vX.Y.Z` son inmutables. Una release anterior no puede reemplazar una versión estable más
  reciente; `nightly` nunca avanza `stable`.
- No existe rama `develop` ni `staging` — los fixes van a `main` primero

### Nix Flakes

```bash
# ejecutar sin instalar
nix run github:themakunga/token-counter/stable

# instalar en el perfil de usuario
nix profile install github:themakunga/token-counter/stable

# actualizar una instalación existente del canal estable
nix profile upgrade token-counter
```

Desde un clon local:

```bash
nix run .        # ejecutar
nix build .      # compila → ./result/bin/token-counter
```

El canal `stable` se crea con la primera release del nuevo pipeline. Para una instalación
reproducible de una versión concreta usa `/vX.Y.Z` en lugar de `/stable`. La rama `main` contiene
desarrollo y no es el canal estable.

Para consumidores como `nix-systems`, la referencia debe ser un tag estable, no un SHA antiguo.
`scripts/update-token-counter.py` consulta la última release publicada, cambia únicamente esa
referencia y actualiza su entrada del lock; el workflow semanal existente incluye ambos cambios en
su PR. Se requiere aplicar ese PR y reconstruir el sistema para actualizar una instalación del
sistema. Un rebuild por sí solo conserva las revisiones de `flake.lock`.

#### Cambios en dependencias de Go

El `flake.nix` incluye un `vendorHash` válido. Si cambian las dependencias, Nix puede señalar el
nuevo hash necesario:

```
error: hash mismatch in fixed-output derivation:
  specified: sha256-AAAA...
  got:       sha256-AbCdEf...   ← copia este valor
```

Reemplaza el valor en `flake.nix`:

```nix
vendorHash = "sha256-AbCdEf...";   # el hash real
```

Luego commitea `flake.nix` y `flake.lock` juntos.

---

### Compilar e instalar desde el repo

```bash
git clone https://github.com/TU-USUARIO/token-counter
cd token-counter
make install          # compila con versión e instala en /usr/local/bin
```

Destino personalizado:

```bash
make install PREFIX=~/.local/bin
```

#### Actualizar a la última versión de `main`

```bash
cd token-counter
make update           # git pull + recompila + reinstala
```

#### Otros comandos

| Comando          | Acción                                                   |
| ---------------- | -------------------------------------------------------- |
| `make build`     | Compila el binario localmente                            |
| `make run`       | Ejecuta sin compilar (`go run .`)                        |
| `make install`   | Compila e instala en `PREFIX` (default `/usr/local/bin`) |
| `make uninstall` | Elimina el binario instalado                             |
| `make update`    | `git pull` + `make install`                              |
| `make dist`      | Cross-compila para darwin/linux × amd64/arm64 en `dist/` |
| `make clean`     | Elimina binario local y carpeta `dist/`                  |

Al primer arranque se crean automáticamente:

- `~/.config/token-counter/config.yaml` — configuración con valores por defecto
- `~/.token-counter.yaml` — plantilla de credenciales (sin secrets, permisos 600)

---

## Fuentes de datos

### Claude Code — archivos locales (sin credenciales)

Lee directamente `~/.claude/projects/**/*.jsonl`. Cada sesión de Claude Code guarda un JSONL con
entradas `type:"assistant"` que incluyen:

```json
{
  "type": "assistant",
  "timestamp": "2026-01-01T10:00:00.000Z",
  "message": {
    "model": "claude-sonnet-4-6",
    "usage": {
      "input_tokens": 1234,
      "output_tokens": 456,
      "cache_creation_input_tokens": 8000,
      "cache_read_input_tokens": 3200
    }
  }
}
```

**Requisito:** tener Claude Code instalado (`~/.claude/projects/` existe). **Sin API key.** El costo
se estima automáticamente según el modelo (sonnet / opus / haiku).

Assistant content blocks are deduplicated by request/message ID across local logs. Repeated
input/cache counts are not added again; the final output count is retained. Token windows remain
rolling, while Claude's USD spend and budget use the current calendar month in the machine's local
timezone.

**Precios aproximados usados (USD por millón de tokens):**

| Familia          | Input   | Output | Cache create | Cache read |
| ---------------- | ------- | ------ | ------------ | ---------- |
| Sonnet           | $3/M    | $15/M  | $3.75/M      | $0.30/M    |
| Opus 4.5–4.8 / 5 | $5/M    | $25/M  | $6.25/M      | $0.50/M    |
| Opus 5.5         | $4/M    | $20/M  | $5/M         | $0.20/M    |
| Opus 4 / 4.1     | $15/M   | $75/M  | $18.75/M     | $1.50/M    |
| Haiku            | $0.80/M | $4/M   | $1.00/M      | $0.08/M    |

> Los precios se detectan por nombre de modelo. Si Anthropic cambia precios, actualiza la función
> `claudeCost` en `providers.go`.

One-hour cache writes use their 2x input rate when reported by Claude Code. The displayed monthly
spend and remaining budget are estimates from retained local logs, including cache charges. They are
not the gateway's official balance.

---

### Codex — local session usage

When `~/.codex/sessions` exists, the existing `openai` entry automatically reads local Codex JSONL
usage instead of querying organization API usage. `id: codex` always uses local sessions;
`CODEX_HOME` is respected. No API key is needed. Set `source: codex` to select local usage
explicitly, or `source: api` to keep tracking the OpenAI API organization separately.

The counter sums changes in cumulative `total_token_usage.total_tokens`, so repeated usage snapshots
are not counted twice. Cached input and reasoning tokens are already included in the reported total
and are not added again. Only locally retained sessions are included; cloud-only or deleted history
is not reconstructed. Hourly, daily, weekly, and monthly columns use rolling 1-hour, 24-hour, 7-day,
and 30-day windows.

Configured token limits remain visible. They are monitoring thresholds, not Codex subscription
limits or enforcement controls. Local logs do not report billed USD, so the spend section displays
`USD unavailable (local usage)`; a configured dollar budget remains visible without inventing
remaining funds. Subscription usage and API billing are separate.

```yaml
agents:
  - id: codex
    name: Codex
    enabled: true
    source: codex
    limits:
      hourly: 50000
      daily: 500000
      weekly: 2000000
      monthly: 8000000
```

### OpenAI — API organization usage (`source: api`)

Usa el endpoint `GET /v1/organization/usage/completions`.

#### Cómo obtener tu API key con permiso de uso

1. Ve a [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
2. Clic en **"Create new secret key"**
3. En **Permissions**, selecciona **"Restricted"**
4. Activa el permiso **"Usage" → Read**
5. Copia la key y agrégala a `~/.token-counter.yaml`

```yaml
agents:
  openai:
    api_key: "sk-proj-..."
```

#### Cómo ver tu consumo en el dashboard de OpenAI

- Panel de uso: [platform.openai.com/usage](https://platform.openai.com/usage)
- API de uso (requiere key con `usage.read`):

```bash
# Uso del día de hoy
curl "https://api.openai.com/v1/organization/usage/completions?start_time=$(date -v-1d +%s)&bucket_width=1d" \
  -H "Authorization: Bearer sk-proj-..."

# Uso del mes actual
curl "https://api.openai.com/v1/organization/usage/completions?start_time=$(date -v-30d +%s)&bucket_width=1d" \
  -H "Authorization: Bearer sk-proj-..."
```

#### Cómo ver costos directamente en dólares

```bash
# Costo del mes (endpoint de costos — misma key con usage.read)
curl "https://api.openai.com/v1/organization/usage/costs?start_time=$(date -v-30d +%s)&bucket_width=1d" \
  -H "Authorization: Bearer sk-proj-..."
```

El costo en el TUI se estima con una tasa promedio de ~$3/M tokens. Para mayor precisión usa el
dashboard.

Sin key, la columna muestra `— unavailable —` junto al límite configurado.

---

### Claude Code vía Vertex AI (gateway corporativo)

For Codeen with a USD 300 monthly allowance, enable only `vertex` (disable the `claude` entry to
avoid counting the same logs twice), set `name: Claude / Codeen` and `monthly_budget: 300`. No
gateway credentials are needed for local log usage. Codeen's startup, telemetry HTTP 200 and
shutdown request counts do not report remaining budget. `stats.jsonl` with zero `accumulated_cost`
cannot establish that usage was free or that all USD 300 remain available.

Para usuarios que acceden a Claude a través de un proxy/gateway local (ej. Cosmos GenAI Gateway en
`localhost:8150`).

**Fuente de datos:** los mismos archivos `~/.claude/projects/**/*.jsonl` que el provider `claude` —
Claude Code los escribe localmente sin importar si el routing es directo o vía Vertex.

**Activar el provider `vertex`** en `~/.config/token-counter/config.yaml`:

```yaml
agents:
  - id: claude
    enabled: false # ← desactivar para no contar dos veces
    name: "Claude Code"
    limits: { ... }

  - id: vertex
    enabled: true
    name: "Claude / Vertex"
    monthly_budget: 50.00
    limits:
      hourly: 100000
      daily: 1000000
      weekly: 5000000
      monthly: 20000000
```

**Agregar credenciales** en `~/.token-counter.yaml` (sensible, no commitear):

```yaml
agents:
  vertex:
    gateway_url: "http://localhost:8150" # URL base del gateway (no sensible)
    project_id: "tu-gcp-project-id" # SENSIBLE — ID del proyecto GCP
```

> ⚠️ El `project_id` es información sensible. Nunca lo incluyas en el repositorio. El archivo
> `~/.token-counter.yaml` debe tener permisos `600` y estar fuera del repo.

---

## Credenciales (`~/.token-counter.yaml`)

```yaml
agents:
  openai:
    api_key: "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXX"

  # vertex:
  #   gateway_url: "http://localhost:8150"
  #   project_id:  "tu-gcp-project-id"    # SENSIBLE
```

Permisos recomendados: `chmod 600 ~/.token-counter.yaml`

---

## Configuración (`~/.config/token-counter/config.yaml`)

Opcional. Si no existe, se usan los valores por defecto.

```yaml
refresh_interval: 30s

agents:
  - id: claude
    name: "Claude Code"
    enabled: true
    monthly_budget: 50.00 # USD — 0 = sin presupuesto configurado
    limits:
      hourly: 100000
      daily: 1000000
      weekly: 5000000
      monthly: 20000000

  - id: openai
    name: "OpenAI / Codex"
    enabled: true
    monthly_budget: 30.00
    limits:
      hourly: 50000
      daily: 500000
      weekly: 2000000
      monthly: 8000000
```

---

## Controles

| Tecla          | Acción         |
| -------------- | -------------- |
| `r`            | Refresh manual |
| `q` / `ctrl+c` | Salir          |
