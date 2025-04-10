# Stock API

Este proyecto es una API para gestionar y procesar recomendaciones de acciones financieras. Proporciona endpoints para consultar información sobre acciones y recomendaciones basadas en datos obtenidos de una API externa.

## Características

- Obtención y almacenamiento de datos de acciones en una base de datos PostgreSQL.
- Procesamiento de recomendaciones con puntajes calculados en base a cambios de rating, precio objetivo, y otros factores.
- Endpoints REST para consultar acciones y recomendaciones.

## Requisitos

- Go 1.24.2 o superior.
- PostgreSQL.
- Conexión a internet para consumir la API externa.

## Configuración

### Variables de entorno

El proyecto utiliza un archivo `.env` para configurar las variables de entorno necesarias. Asegúrate de crear este archivo en la raíz del proyecto con el siguiente contenido:

```env
DB_URL="postgresql://<usuario>:<contraseña>@<host>:<puerto>/<base_de_datos>?sslmode=disable"
API_KEY="<tu_api_key>"
PORT="8081"