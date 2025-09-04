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

