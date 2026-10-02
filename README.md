# token-counter

Terminal TUI que muestra el uso de tokens de tus agentes de IA en tiempo real.

```
╭──────────────────────────────────────────────────────────────────────────────╮
│  Agent             Hourly                    Daily                    Weekly  ⬤ │
│  ──────────────────────────────────────────────────────────────────────────  │
│  Claude Code       1.2K / 100K               45.6K / 1M              234K / 5M  ● │
│  OpenAI / Codex    2.3K / 50K                89K / 500K              1.2M / 2M  ● │
│                                                                               │
│  last: 10:30:45  •  r=refresh  q=quit                                        │
╰──────────────────────────────────────────────────────────────────────────────╯
```

**Colores:** 🟢 < 50% del límite · 🟡 50–80% · 🔴 > 80%

---

## Instalación

```bash
cd token-counter
go mod tidy
go build -o token-counter .
# opcional: mover al PATH
mv token-counter /usr/local/bin/
```

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
**Sin API key.** El conteo incluye tokens de caché (creación + lectura).

---

### OpenAI / Codex — API de uso

Usa el endpoint `GET /v1/organization/usage/completions`.

**Requisito:** API key con scope `usage.read`.  
Genera una en [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
seleccionando el permiso **"Usage" → Read**.

Agrégala a `~/.token-counter.yaml`:

```yaml
agents:
  openai:
    api_key: "sk-proj-..."
```

Sin key, la columna muestra `— no key —` sin bloquear la app.

---

## Credenciales (`~/.token-counter.yaml`)

```yaml
agents:
  openai:
    api_key: "sk-proj-XXXXXXXXXXXXXXXXXXXXXXXX"

  # ejemplo agente custom (ver sección Configuración)
  mi-agente:
    api_key: "mi-key-secreta"
```

Permisos recomendados: `chmod 600 ~/.token-counter.yaml`

---

## Configuración (`~/.config/token-counter/config.yaml`)

Opcional. Si no existe, se usan los valores por defecto.

```yaml
refresh_interval: 30s   # cuánto esperar entre actualizaciones

agents:
  - id: claude
    name: "Claude Code"
    enabled: true
    limits:
      hourly: 100000
      daily: 1000000
      weekly: 5000000

  - id: openai
    name: "OpenAI / Codex"
    enabled: true
    limits:
      hourly: 50000
      daily: 500000
      weekly: 2000000
```

### Agregar más agentes

Para soportar un proveedor nuevo, agrega su `id` en el config y luego
implementa la interfaz `Provider` en `providers.go`:

```go
type Provider interface {
    Name() string
    Fetch(ctx context.Context) ([]PeriodUsage, error)
}
```

Registra tu proveedor en `buildProviders()` con un `case "mi-id":`.

---

## Controles

| Tecla | Acción |
|-------|--------|
| `r` | Refresh manual |
| `q` / `ctrl+c` | Salir |
