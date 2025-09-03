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
