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
