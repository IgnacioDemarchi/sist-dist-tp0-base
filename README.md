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
