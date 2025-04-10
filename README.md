# 📈 Stock API

Este proyecto es una API para gestionar y procesar recomendaciones de acciones financieras. Proporciona endpoints para consultar información sobre acciones y recomendaciones basadas en datos obtenidos de una API externa.

## ✨ Características

- 📊 Obtención y almacenamiento de datos de acciones en una base de datos PostgreSQL.
- 🧠 Procesamiento de recomendaciones con puntajes calculados en base a cambios de rating, precio objetivo, y otros factores.
- 🔌 Endpoints REST para consultar acciones y recomendaciones.

## 🧰 Requisitos

- 🟦 Go 1.24.2 o superior.
- 🐘 PostgreSQL.
- 🌐 Conexión a internet para consumir la API externa.

## ⚙️ Configuración

### 🔐 Variables de entorno

El proyecto utiliza un archivo `.env` para configurar las variables de entorno necesarias. Asegúrate de crear este archivo en la raíz del proyecto con el siguiente contenido:

```env
DB_URL="postgresql://<usuario>:<contraseña>@<host>:<puerto>/<base_de_datos>?sslmode=disable"
API_KEY="<tu_api_key>"
PORT="8081"
🔑 API_KEY: Clave de acceso para la API externa.

🌐 PORT: Puerto en el que se ejecutará la API.

🚀 Primeros Pasos
📥 Clona este repositorio:

bash
Copiar
Editar
git clone https://github.com/JuanVel1/stock-api.git
cd stock-api
📦 Instala las dependencias:

bash
Copiar
Editar
go mod tidy
▶️ Ejecución
Inicia la aplicación:

bash
Copiar
Editar
go run main.go
La API estará disponible en:
🔗 http://localhost:<PORT> (por defecto, en el puerto 8081)

📡 Endpoints disponibles
GET /api/stocks → 📁 Devuelve una lista de acciones almacenadas en la base de datos.

GET /api/recommendations → ⭐ Devuelve las mejores recomendaciones procesadas.