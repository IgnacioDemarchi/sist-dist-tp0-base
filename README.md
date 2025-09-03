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
