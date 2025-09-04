### Ejercicio 7 – Notificación de fin y consulta de ganadores por agencia

Se extendieron **cliente** y **servidor** para coordinar el **sorteo**:  
cada cliente notifica que terminó de enviar sus apuestas y, luego del sorteo global, consulta los **ganadores de su propia agencia**.

#### Ejecución

1) **Levantar servicios** (se asume 5 agencias/clients):
```bash
make docker-compose-up
```

2) **(Opcional) Configurar cantidad esperada de agencias** en el servidor:  
el servidor puede inferirla dinámicamente a partir de los `DONE` recibidos (IDs 1..N).  
Si se prefiere fijarla explícitamente:
```bash
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
  Se mantiene el framing binario de 4 bytes **big-endian** + **JSON**.
  - Cliente → Servidor (**DONE**): avisa que finalizó el envío de apuestas.
    ```json
    { "type": "DONE", "agency_id": "3" }
    ```
    Respuesta:
    ```json
    { "type": "ACK_DONE", "ok": true }
    ```
  - Cliente → Servidor (**GET_WINNERS**): solicita ganadores para su agencia.  
    Antes del sorteo, el servidor responde no disponible.
    ```json
    { "type": "GET_WINNERS", "agency_id": "3" }
    ```
    Respuesta antes del sorteo:
    ```json
    { "type": "WINNERS", "ok": false, "error": "not_ready" }
    ```
    Respuesta post-sorteo:
    ```json
    { "type": "WINNERS", "ok": true, "dnis": ["30904465","..."] }
    ```

- **Comportamiento del servidor**
  - Guarda apuestas (BET / BATCH_BET) como en ej6.
  - Registra `DONE` por `agency_id`.  
  - Cuando recibió `DONE` de **todas** las agencias (N = `CLIENT_AMOUNT` o inferido 1..N), ejecuta el sorteo:
    - Carga apuestas con `load_bets(...)`.
    - Evalúa ganadores con `has_won(...)`.
    - Agrupa por **agencia** y guarda en memoria (`_winners_by_agency`).
    - Log: `action: sorteo | result: success`.
  - Solo después del sorteo responde **WINNERS { ok:true }** con los **DNI** de la agencia solicitante (sin broadcast ni filtraciones).

- **Comportamiento del cliente**
  - Tras enviar todas sus apuestas (en *batches* como ej6), envía **DONE**.
  - Luego **polling** ligero (backoff corto) con **GET_WINNERS** hasta obtener `ok:true`.
  - Log final por agencia:  
    `action: consulta_ganadores | result: success | cant_ganadores: <CANT>`.
  - Conserva cierre *graceful* (SIGTERM) y reconexiones por operación.

- **Aislamiento por agencia**
  - El servidor devuelve **solo** los **DNI de ganadores** de la **agencia solicitante**.  
  - No hay broadcast masivo ni exposición de ganadores de otras agencias.

- **Manejo robusto de red**
  - Se evita **short read/write** con `sendall`/lecturas exactas en Python y `io.ReadFull`/envíos completos en Go.
  - Validación de tipos y control de errores en cada handler.
  - Reintentos del cliente ante `not_ready` hasta que el sorteo se complete.

