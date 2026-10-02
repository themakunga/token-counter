# token-counter

Terminal TUI que muestra el uso de tokens y costo mensual estimado de tus agentes de IA en tiempo real.

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

**Colores:** 🟢 < 50% del límite · 🟡 50–80% · 🔴 > 80%  
**Monthly spend:** colorea según porcentaje del presupuesto configurado.

---

## Descarga

### Binarios pre-compilados (recomendado)

| Canal | Cuándo usar | Link |
|-------|-------------|------|
| **Estable** | Producción / uso diario | [Última versión](../../releases/latest) |
| **Nightly** | Probar cambios recientes de `main` | [nightly](../../releases/tag/nightly) |

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
- No existe rama `develop` ni `staging` — los fixes van a `main` primero

### Compilar desde fuente

```bash
git clone https://github.com/TU-USUARIO/token-counter
cd token-counter
go build -ldflags="-X main.version=$(git describe --tags --always)" -o token-counter .
mv token-counter /usr/local/bin/
```

Al primer arranque se crean automáticamente:
- `~/.config/token-counter/config.yaml` — configuración con valores por defecto
- `~/.token-counter.yaml` — plantilla de credenciales (sin secrets, permisos 600)

---

## Fuentes de datos

### Claude Code — archivos locales (sin credenciales)

Lee directamente `~/.claude/projects/**/*.jsonl`. Cada sesión de Claude Code
guarda un JSONL con entradas `type:"assistant"` que incluyen:

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

**Requisito:** tener Claude Code instalado (`~/.claude/projects/` existe).  
**Sin API key.** El costo se estima automáticamente según el modelo (sonnet / opus / haiku).

**Precios aproximados usados:**

| Familia  | Input   | Output   | Cache create | Cache read |
|----------|---------|----------|--------------|------------|
| Sonnet   | $3/M    | $15/M    | $3.75/M      | $0.30/M    |
| Opus     | $15/M   | $75/M    | $18.75/M     | $1.50/M    |
| Haiku    | $0.80/M | $4/M     | $1.00/M      | $0.08/M    |

> Los precios se detectan por nombre de modelo. Si Anthropic cambia precios, actualiza la función `claudeCost` en `providers.go`.

---

### OpenAI / Codex — API de uso

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

El costo en el TUI se estima con una tasa promedio de ~$3/M tokens. Para mayor precisión usa el dashboard.

Sin key, la columna muestra `— no key —` sin bloquear la app.

---

### Codeen — proxy corporativo de IA

> **Sección en construcción.** Codeen es la plataforma interna que enruta tus requests a modelos de IA.

Para agregar soporte, necesito saber:
1. ¿Hay una URL de API para consultar métricas de uso? (ej. `/v1/usage` o similar)
2. ¿Cómo se autentica? (token, cookie, SSO)
3. ¿Qué formato retorna? (tokens, costo, ambos)

Una vez definido, se implementa un nuevo `Provider` en `providers.go`:

```go
type codeenProvider struct{ cfg AgentConfig; apiKey string }

func (c *codeenProvider) Name() string { return c.cfg.Name }

func (c *codeenProvider) Fetch(ctx context.Context) ([]PeriodUsage, error) {
    // TODO: llamar al endpoint de Codeen
}
```

Y se registra en `buildProviders()` con `case "codeen":`.

---

## Credenciales (`~/.token-counter.yaml`)

```yaml
agents:
  openai:
    api_key: "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXX"

  # codeen:
  #   api_key: "tu-token-de-codeen"
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
    monthly_budget: 50.00   # USD — 0 = sin presupuesto configurado
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

  # - id: codeen
  #   name: "Codeen"
  #   enabled: true
  #   monthly_budget: 0
  #   limits:
  #     hourly: 0
  #     daily: 0
  #     weekly: 0
  #     monthly: 0
```

---

## Controles

| Tecla | Acción |
|-------|--------|
| `r` | Refresh manual |
| `q` / `ctrl+c` | Salir |
