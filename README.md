### Ejercicio 6 – Procesamiento por *batches* (chunks) desde datasets

Se extendió el cliente para enviar **varias apuestas por consulta** (modalidad *batch*), y el servidor para **aceptar y persistir lotes completos**.  
Se mantiene el protocolo con **framing binario (4 bytes big-endian)** y **líneas delimitadas por `\n`**, respetando el límite de **8 kB** por frame.

---

#### Ejecución

1) **Preparar datasets** (provistos por la cátedra):

```bash
# Colocar y descomprimir dentro del repo
unzip .data/datasets.zip -d .data/

# Deben existir:
#   .data/agency-1.csv
#   .data/agency-2.csv
#   ...
#   .data/agency-5.csv
```

2) **Levantar servicios** (el compose ya monta datasets y config):

```bash
make docker-compose-up
```

3) **Ver logs**:

```bash
make docker-compose-logs
```

- Cliente (lote enviado OK):

```
action: apuesta_enviada | result: success | cantidad: <N>
```

- Servidor (lote almacenado OK):

```
action: apuesta_recibida | result: success | cantidad: <N>
```

- Si hay **algún error** en el lote, el servidor responde **NACK** y loguea:

```
action: apuesta_recibida | result: fail | cantidad: <N>
```

---

#### Detalles importantes de la solución

- **Ingesta por agencia via datasets**  
  Cada cliente `N` carga su archivo `/.data/agency-{N}.csv`, inyectado por **volumen**:

```yaml
# en docker-compose
volumes:
  - ./.data:/data:ro
```

  El cliente lee `/data/agency-{ID}.csv`, parsea filas y arma apuestas.

- **Batch configurable y ≤ 8 kB**  
  En `client/config.yaml` se respeta la clave:

```yaml
batch:
  maxAmount: 100   # ejemplo; ajustar para que cada batch no supere 8 kB
```

  Además del tope lógico `batch.maxAmount`, el cliente **fragmenta** dinámicamente  
  (particionado binario) para garantizar que **cada frame ≤ 8192 bytes**.  
  Si un chunk no entra, lo divide y envía sub-lotes sucesivos.

- **Protocolo y mensajes**
  - Framing: **4 bytes** de longitud **big-endian** + texto UTF-8 con `\n`.
  - **Solicitud de batch** (cliente → servidor):

    ```
    BATCH|<agency_id>|<cantidad>\n
    BET|<agency_id>|<nombre>|<apellido>|<documento>|<nacimiento>|<numero>\n
    BET|<agency_id>|<nombre>|<apellido>|<documento>|<nacimiento>|<numero>\n
    ...
    ```

  - **Respuesta** (servidor → cliente):

    ```
    ACK_BATCH|OK|<N>\n
    ```

    o, en error:

    ```
    ACK_BATCH|ERR|<N>|<motivo>\n
    ```

- **Servidor (Python)**
  - En `__handle_client_connection` detecta `BATCH`, construye `Bet(...)` por cada fila  
    y **persiste atómicamente** con `store_bets(bets)`.  
  - Log de éxito:  
    ```
    action: apuesta_recibida | result: success | cantidad: N
    ```  
  - En caso de excepción, responde `ACK_BATCH|ERR` y loguea `result: fail`.

- **Cliente (Go)**
  - Carga CSV de la agencia (`/data/agency-{ID}.csv`).  
  - Arma *batches* de tamaño ≤ `batch.maxAmount` y además aplica **particionado por tamaño** (≤ 8 kB/frame).  
  - Envía lote(s) con `SendBatches(...)`; valida `ACK_BATCH`.  
  - Logs por lote enviado (éxito/fallo) y cierre *graceful* heredado (SIGTERM).

- **Short read/write evitados**
  - **Python**: `sendall`, lecturas exactas (`_recv_exact`).  
  - **Go**: `io.ReadFull` y `sendall` equivalente, más validaciones de tamaño.

---

#### Notas de composición (ya incorporadas en el script)

- Cada `clientN` monta:

```yaml
- ./client/config.yaml:/config.yaml:ro
- ./.data:/data:ro
```

- Variables de ejemplo (además de `CLI_ID`) para compatibilidad con ejercicios previos:

```yaml
- NOMBRE=JuanN
- APELLIDO=PerezN
- DOCUMENTO=<random>
- NACIMIENTO=1990-01-01
- NUMERO=<random>
```

*(El ejercicio 6 prioriza los CSV; estas envs pueden quedar para backwards-compat.)*

---

#### Pruebas rápidas

- **OK**: datasets válidos, `batch.maxAmount` razonable → ver `success` en cliente y servidor.  
- **Error en lote**: forzar un registro inválido en el CSV (ej. `numero` no entero) →  
  el servidor debe responder `ACK_BATCH|ERR` y loguear `result: fail`.
