# Trabajo Práctico – Sistemas Distribuidos (tp0)

---

### Ejercicio 1 – Generación de `docker-compose`

Para este ejercicio se desarrolló un script en **Bash** (`generar-compose.sh`) que genera un archivo `docker-compose` con una configuración predeterminada para un servidor y un número configurable de clientes.  

#### Ejecución

```bash
./generar-compose.sh [archivo_salida] [n_clientes]
```

- **archivo_salida**: nombre del archivo de salida (por defecto `docker-compose-dev.yaml`)  
- **n_clientes**: cantidad de clientes a generar (por defecto `5`)  

Ejemplo:

```bash
./generar-compose.sh docker-compose-dev.yaml 3
```

#### Detalles importantes de la solución

- Se utiliza **heredoc** (`cat <<EOF`) para escribir bloques YAML de forma clara y mantener la indentación correcta.  
- El script crea la definición del servicio `server` y, en un bucle, agrega `client1`, `client2`, … hasta `n_clientes`.  
- Cada cliente tiene su propia variable de entorno `CLI_ID` para diferenciarse.  
- Se define una red personalizada (`testing_net`) con un **subnet** fijo para asegurar conectividad predecible.  
- El uso de `set -euo pipefail` hace el script más robusto, evitando fallos silenciosos.  

---

### Ejercicio 2 – Montaje de configuraciones con volúmenes

En este ejercicio se amplió el script `generar-compose.sh` para incluir el montaje de archivos de configuración en el contenedor del servidor y de los clientes mediante volúmenes.  

#### Ejecución

```bash
./generar-compose.sh [archivo_salida] [n_clientes]
```

- **archivo_salida**: nombre del archivo de salida (por defecto `docker-compose-dev.yaml`)  
- **n_clientes**: cantidad de clientes a generar (por defecto `5`)  

Ejemplo:

```bash
./generar-compose.sh docker-compose-dev.yaml 3
```

#### Detalles importantes de la solución

- El servicio `server` monta el archivo `./server/config.ini` en el contenedor, en modo **solo lectura**, para centralizar su configuración.  
- Cada cliente monta `./client/config.yaml` también en modo **solo lectura**, de forma que todos acceden a la misma configuración base.  
- Se mantiene la lógica de generación dinámica de clientes con identificadores (`CLI_ID`).  
- La red `testing_net` con **subnet** fijo sigue asegurando conectividad controlada entre los contenedores.  
- Al usar volúmenes se facilita modificar la configuración sin necesidad de reconstruir imágenes.  

---

### Ejercicio 3 – Prueba del servidor Echo

En este ejercicio se desarrolló un script en **Shell** (`test_echo_server.sh`) que valida el correcto funcionamiento del servidor echo desplegado con `docker-compose`.  

#### Ejecución

```bash
./test_echo_server.sh [host] [port] [mensaje]
```

- **host**: nombre del contenedor o servicio del servidor (por defecto `server`)  
- **port**: puerto en el que escucha el servidor (por defecto `12345`)  
- **mensaje**: texto a enviar al servidor (por defecto se genera automáticamente con PID y timestamp)  

Ejemplo:

```bash
./test_echo_server.sh server 12345 "hola mundo"
```

#### Detalles importantes de la solución

- El script detecta automáticamente la red a la que está conectado el servidor a través de `docker inspect`.  
- Se ejecuta un contenedor temporal de **BusyBox** en esa red para enviar el mensaje mediante `nc`.  
- Se usa un timeout configurable (`NC_TIMEOUT`, por defecto `3s`) para evitar bloqueos.  
- El mensaje de respuesta se compara con el original (ignorando saltos de línea).  
- Si el servidor responde correctamente, se imprime:  
  ```
  action: test_echo_server | result: success
  ```  
  En caso contrario, se imprime:  
  ```
  action: test_echo_server | result: fail
  ```  

---

### Ejercicio 4 – Finalización graceful con SIGTERM

En este ejercicio se modificaron **cliente** y **servidor** para que ambos finalicen de manera *graceful* al recibir la señal `SIGTERM`.  
Finalizar de forma graceful implica cerrar correctamente todos los *file descriptors* (sockets, conexiones, etc.) antes de que el proceso principal termine.  

#### Ejecución

El cliente y servidor se ejecutan normalmente con `docker-compose`. Para probar la finalización graceful:  

```bash
docker compose up
docker compose down -t 5
```

El flag `-t` indica el tiempo de espera (en segundos) para que los contenedores reciban la señal `SIGTERM` y finalicen de forma ordenada antes de forzar un `SIGKILL`.  

#### Detalles importantes de la solución

- **Cliente (Go):**
  - Se añadió un `signal.Notify` que captura `SIGINT` y `SIGTERM`.  
  - Al recibir la señal, se invoca `client.Close()`, que:
    - Cierra el canal `stopCh` para indicar la detención del loop.  
    - Cierra el socket activo para desbloquear cualquier `read/write`.  
  - Los mensajes de log confirman el cierre del loop y de la conexión.  

- **Servidor (Python):**
  - Se registraron handlers para `SIGTERM` y `SIGINT`.  
  - En el handler se llama a `server.stop()`, que:
    - Cierra el socket de escucha.  
    - Marca la bandera `_stopping` para salir del loop principal.  
  - El servidor loguea el inicio y éxito del proceso de cierre.  

- Ambos sistemas reportan en logs la finalización de recursos al recibir la señal, garantizando un cierre controlado y evitando fugas de recursos.  

---

### Ejercicio 5 – Quiniela: protocolo, serialización y persistencia

En este ejercicio se modificó la lógica del **cliente** y el **servidor** para modelar el caso de uso de una agencia de quiniela que registra apuestas en una central (Lotería Nacional). Se incorporó un **módulo de comunicación** con protocolo propio, serialización en texto delimitado y manejo robusto de sockets.

#### Ejecución

1) **Levantar servicios** (5 agencias/clientes como ejemplo):
```bash
make docker-compose-up
```

2) **Configurar variables de entorno por cliente** (ejemplo para una agencia):

#### Asignar por contenedor/servicio del cliente
```bash
NOMBRE="Santiago Lionel"
APELLIDO="Lorca"
DOCUMENTO="30904465"
NACIMIENTO="1999-03-17"
NUMERO="7574"
```

Cada cliente (agencia 1..5) debe tener sus propios valores. El cliente toma estos campos de `ENV` y los envía al servidor.

3) **Ver logs**:
```bash
make docker-compose-logs
```

- Cliente (éxito):
```
action: apuesta_enviada | result: success | dni: 30904465 | numero: 7574
```

- Servidor (persistencia ok):
```
action: apuesta_almacenada | result: success | dni: 30904465 | numero: 7574
```

#### Detalles importantes de la solución

- **Variables de entorno (cliente)**  
  - `NOMBRE`, `APELLIDO`, `DOCUMENTO`, `NACIMIENTO` (`YYYY-MM-DD`), `NUMERO` (entero).  
  - El cliente arma la apuesta con la `AgencyID` (ID del cliente) y envía el mensaje.

- **Protocolo y serialización (módulo de comunicación)**  
  - **Framing** binario con **prefijo de longitud** de 4 bytes **big-endian** (`!I`).  
  - **Carga** serializada como línea delimitada por `|`:  
    - Solicitud (**BET**):  
      `BET|<agency_id>|<nombre>|<apellido>|<documento>|<nacimiento>|<numero>\n`
    - Respuesta (**ACK**):  
      `ACK|OK\n`  
      o en caso de error:  
      `ACK|ERR|<razón>\n`  
  - Funciones helper:  
    - **Python**: `send_line/recv_line` (evitan short read/write usando `sendall` y lecturas exactas).  
    - **Go**: `writeFrame/readFrame` con `io.ReadFull` + helpers de encoding/decoding de líneas.

- **Servidor (Python)**  
  - Acepta conexiones, recibe **BET**, mapea a `Bet(...)` y persiste con `store_bets([...])` (provista por la cátedra).  
  - Responde `ACK|OK` o `ACK|ERR|<razón>`.  
  - Manejo de errores y límites (p. ej., tamaño máximo de frame), logs de recepción y persistencia.  
  - Conserva el cierre *graceful* (SIGINT/SIGTERM) del ejercicio anterior.

- **Cliente (Go)**  
  - Lee `ENV`, construye `Bet`, invoca `SendBet`, valida **ACK** y loguea el resultado.  
  - Loop interrumpible y cierre *graceful* (canal `stopCh` + cierre de socket) ante `SIGTERM`.  
  - Logs de éxito/fracaso por apuesta.

- **Separación de responsabilidades**  
  - **Dominio**: `Bet` / `store_bets` (servidor).  
  - **Comunicación**: framing + (de)serialización en líneas.  
  - **Aplicación**: orquestación, validación, logs y control de ciclo de vida.

- **Manejo robusto de sockets**  
  - Prevención de **short read/write** (`sendall`/`io.ReadFull`/lecturas exactas).  
  - Validación de tamaños (rechazo de frames inválidos).  
  - Cierre seguro de conexiones en `finally/defer` y ante señales.

---

### Ejercicio 6 – Procesamiento por *batches* (chunks) desde datasets

Se extendió el cliente para enviar **varias apuestas por consulta** (modalidad *batch*), y el servidor para **aceptar y persistir lotes completos**.  
Se mantiene el protocolo con **framing binario (4 bytes big-endian)** y **líneas delimitadas por `\n`**, respetando el límite de **8 kB** por frame.

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

#### Pruebas rápidas

- **OK**: datasets válidos, `batch.maxAmount` razonable → ver `success` en cliente y servidor.  
- **Error en lote**: forzar un registro inválido en el CSV (ej. `numero` no entero) →  
  el servidor debe responder `ACK_BATCH|ERR` y loguear `result: fail`.

---

### Ejercicio 7 – Notificación de fin y consulta de ganadores por agencia

Se extendieron **cliente** y **servidor** para coordinar el **sorteo**:  
cada cliente notifica que terminó de enviar sus apuestas y, luego del sorteo global, consulta los **ganadores de su propia agencia**.

#### Ejecución

1) **Levantar servicios** (se asume 5 agencias/clientes):

```bash
make docker-compose-up
```

2) **(Opcional) Configurar cantidad esperada de agencias en el servidor**:  
El servidor puede inferirla dinámicamente a partir de los `DONE` recibidos (IDs 1..N).  
Si se prefiere fijarla explícitamente:

```yaml
# en el servicio del servidor
environment:
  - CLIENT_AMOUNT=5
```

3) **Ver logs**:

```bash
make docker-compose-logs
```

- Cuando todas las agencias notifican fin:

```
action: sorteo | result: success
```

- Cliente (tras obtener su lista):

```
action: consulta_ganadores | result: success | cant_ganadores: <CANT>
```

#### Detalles importantes de la solución

- **Protocolo (mensajes nuevos)**  
  Se mantiene el framing binario de 4 bytes **big-endian** + líneas UTF-8 delimitadas por `\n`.

  - Cliente → Servidor (**DONE**): avisa que finalizó el envío de apuestas.
    ```
    DONE|<agency_id>\n
    ```
    Respuesta:
    ```
    ACK_DONE|OK\n
    ```

  - Cliente → Servidor (**GET_WINNERS**): solicita ganadores para su agencia.  
    Antes del sorteo, el servidor responde no disponible.
    ```
    GET_WINNERS|<agency_id>\n
    ```
    Respuesta antes del sorteo:
    ```
    WINNERS|ERR|not_ready\n
    ```
    Respuesta post-sorteo (lista de DNIs separados por coma, puede ser vacía):
    ```
    WINNERS|OK|30904465,12345678,...\n
    ```

- **Comportamiento del servidor**
  - Guarda apuestas (BET / BATCH) como en ej6.  
  - Registra `DONE` por `agency_id`.  
  - Cuando recibió `DONE` de todas las agencias (N = `CLIENT_AMOUNT` o inferido 1..N), ejecuta el sorteo:
    - Carga apuestas con `load_bets(...)`.  
    - Evalúa ganadores con `has_won(...)`.  
    - Agrupa por agencia y guarda en memoria (`_winners_by_agency`).  
    - Log: `action: sorteo | result: success`.  
  - Solo después del sorteo responde `WINNERS|OK|...` con los DNI de la agencia solicitante (sin broadcast ni filtraciones).

- **Comportamiento del cliente**
  - Tras enviar todas sus apuestas (en *batches* como ej6), envía `DONE`.  
  - Luego hace *polling* con `GET_WINNERS` hasta obtener `WINNERS|OK`.  
  - Log final por agencia:
    ```
    action: consulta_ganadores | result: success | cant_ganadores: <CANT>
    ```
  - Conserva cierre *graceful* (SIGTERM) y reconexiones por operación.

- **Aislamiento por agencia**
  - El servidor devuelve **solo** los DNIs de ganadores de la agencia solicitante.  
  - No hay broadcast masivo ni exposición de ganadores de otras agencias.

- **Manejo robusto de red**
  - Se evita *short read/write* con `sendall` / lecturas exactas en Python y `io.ReadFull` / envíos completos en Go.  
  - Validación de tipos y control de errores en cada handler.  
  - Reintentos del cliente ante `not_ready` hasta que el sorteo se complete.

---

### Ejercicio 8 – Concurrencia en el servidor (multi-thread)

Se modificó el **servidor (Python)** para **aceptar múltiples conexiones y procesar mensajes en paralelo** mediante *threads*, cuidando la **sincronización** alrededor de la persistencia y del estado global del sorteo.

#### Ejecución

```bash
make docker-compose-up
make docker-compose-logs
```

> Opcional: fijar la cantidad esperada de agencias para el sorteo
```yaml
# en el servicio del servidor
environment:
  - CLIENT_AMOUNT=5
```

#### Qué cambia

- **Aceptación concurrente**: por cada `accept()` se lanza un **worker thread** que corre `__handle_client_connection`.
- **Límite de workers**: se mantiene un set de threads vivos y un tope (`_max_workers = 64`) para evitar sobrecarga.
- **Sincronización**:
  - Se usa un **`threading.RLock()`** (`_lock`) para proteger acceso a:
    - Estados globales: `_done_agencies`, `_expected_agencies`, `_draw_completed`, `_winners_by_agency`, `_workers`.
    - **Persistencia**: llamadas a `store_bets([...])` (y `load_bets()` durante el sorteo) se hacen **bajo lock** para garantizar atomicidad/consistencia.
  - Handlers de mensajes que mutan estado (p. ej., `DONE`, `GET_WINNERS`, `BATCH_BET`, `BET`) coordinan con el lock.
- **Aceptación no bloqueante**: el socket de escucha tiene `settimeout(0.5)`; esto permite:
  - *Reap* periódico de threads terminados (`_reap_workers()`).
  - Salir ordenadamente cuando `_stopping` es `True`.
- **Cierre graceful**: en `stop()` se cierra el socket de escucha (desbloquea `accept()`) y se hace **best-effort `join()`** de los workers.

#### Consideraciones sobre Python (GIL)

- El **GIL** limita la ejecución verdadera en paralelo de **CPU-bound**; sin embargo, este servidor es **I/O-bound** (sockets), por lo que los *threads* permiten **superponer I/O** y **atender múltiples clientes** concurrentemente.
- Aun así, las secciones críticas (persistencia y estado del sorteo) requieren **lock** para:
  - Evitar *race conditions* en `store_bets`/`load_bets`/`has_won`.
  - Entregar respuestas consistentes (p. ej., `WINNERS` solo *post-sorteo*).

#### Protocolo y handlers (resumen)

- Se mantienen los tipos de mensaje de ejercicios previos: `BET`, `BATCH`, `DONE`, `GET_WINNERS`.  
- **Persistencia (concurrencia segura)**:
  - `BET`: `store_bets([bet])` protegido con **lock** → `ACK|OK`.
  - `BATCH`: `store_bets(bets)` protegido con **lock**; log `apuesta_recibida` (success/fail) → `ACK_BATCH|OK|N` o `ACK_BATCH|ERR|...`.
- **Coordinación de sorteo (multithreading)**:
  - `DONE` agrega la agencia a `_done_agencies` bajo lock.  
  - Si `CLIENT_AMOUNT` no está fijado, se **infieren** agencias 1..N a partir de los `DONE`.  
  - `_perform_draw_if_ready()` (bajo lock) ejecuta el sorteo cuando se alcanzó N:
    - `load_bets()` + `has_won()` → `_winners_by_agency`.  
    - Log: `action: sorteo | result: success`.
- **Consulta de ganadores**:
  - `GET_WINNERS`:
    - Si el sorteo no terminó → `WINNERS|ERR|not_ready`.
    - Si terminó → `WINNERS|OK|dni1,dni2,...` **solo de la agencia solicitante**.

#### Por qué `RLock` y no `Lock`

- Algunos métodos internos (como `_perform_draw_if_ready`) se invocan desde secciones ya protegidas; `RLock` evita *self-deadlock* si se vuelve a adquirir el lock en el mismo hilo.

#### Pruebas recomendadas

- **Carga concurrente**: levantar 5 clientes enviando *batches* en paralelo; verificar throughput y que no haya `race conditions` (logs consistentes y sin errores de persistencia).
- **Sorteo**: enviar `DONE` desde cada agencia; confirmar `action: sorteo | result: success` una sola vez.
- **`GET_WINNERS` antes del sorteo**: debe responder `not_ready`.
- **Tope de workers**: reducir `_max_workers` a un valor pequeño y abrir muchas conexiones breves; verificar que el servidor cierre nuevas conexiones cuando se alcanza el límite (degradación controlada).

## Apéndice técnico (ej6–ej8)

### 1) Carga de CSV por *chunks*
- **Motivo:** evitar cargar archivos grandes (p.ej. 16 GB) en RAM.
- **Estrategia:** lectura incremental con `csv.Reader.Read()` y **emisión por lotes** de hasta `BatchMax` filas.
- **Ventajas:** memoria acotada ≈ `O(BatchMax)`, backpressure natural hacia la red/servidor.
- **Pseudocódigo (Go):**
```go
func streamAgencyBets(agencyID string, maxChunk int, emit func([]Bet) error) error {
    r := csv.NewReader(file)
    // detectar header livianamente
    first, _ := r.Read()
    if !isHeader(first) { enqueue(first) }
    buf := make([]Bet, 0, maxChunk)
    for {
        row, err := r.Read()
        if err == io.EOF { if len(buf)>0 { emit(buf) }; return nil }
        if err != nil { return err }
        if bet, ok := parse(row); ok { buf = append(buf, bet) }
        if len(buf) == maxChunk { if err := emit(buf); err != nil { return err }; buf = buf[:0] }
    }
}
```

### 2) Evitar “double writes” en el framing
- **Motivo:** minimizar syscalls y estados intermedios inconsistentes (header y cuerpo por separado).
- **Estrategia:** construir un solo buffer `4+len(payload)` y escribirlo con una llamada (con lazo anti short-write).
- **Implementación (Go):**
```go
func writeFrame(conn net.Conn, payload []byte) error {
    if len(payload) > 8*1024 { return fmt.Errorf("payload too big: %d", len(payload)) }
    buf := make([]byte, 4+len(payload))
    binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
    copy(buf[4:], payload)
    for off := 0; off < len(buf); {
        n, err := conn.Write(buf[off:])
        if err != nil { return err }
        off += n
    }
    return nil
}
```
> Nota: también se podría usar `syscall.Writev` en Linux, pero el enfoque anterior es portable y claro.

### 3) Locks para persistencia y estado (Servidor Python)
- **Motivo:** `store_bets()/load_bets()` no son thread-safe y el servidor atiende conexiones concurrentes.
- **Estrategia de bloqueo:**
  - `persist_lock`: serializa llamadas a `store_bets`/`load_bets`.
  - `state_lock`: protege `_done_agencies`, `_draw_completed`, `_winners_by_agency`.
  - `workers_lock`: administra el set de threads trabajadores (alta/baja y *reaping*).
- **Patrones:**
  - Al guardar apuestas (BET/BATCH): `with persist_lock: store_bets(...)`.
  - Al correr el sorteo: *snapshot* bajo `persist_lock`, publicación de resultados bajo `state_lock`.

### 4) Terminación graciosa con SIGTERM (Servidor)
- **Motivo:** finalizar sin cortar escrituras/lecturas a mitad de protocolo ni corromper el almacenamiento.
- **Flujo:**
  1. `main` registra handler de SIGTERM → llama `server.stop()`.
  2. `server.stop()`:
     - marca `_stopping = True`;
     - cierra el socket de *listen* (desbloquea `accept()`);
     - hace *best-effort* `join` de workers.
  3. *Loop* principal sale; workers terminan al finalizar su petición o al detectar EOF/cierre.
- **Log compatible con los tests:**
  ```
  action: shutdown | result: in_progress
  ```
  al recibir la señal y no usar valores fuera de `{success, fail, in_progress}` (p.ej., evitar “installed”).
