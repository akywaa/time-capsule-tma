# TIME CAPSULE (TELEGRAM MINI APP)

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=for-the-badge&logo=go&logoColor=white) ![MongoDB](https://img.shields.io/badge/MongoDB-%234ea94b.svg?style=for-the-badge&logo=mongodb&logoColor=white) ![JavaScript](https://img.shields.io/badge/javascript-%23323330.svg?style=for-the-badge&logo=javascript&logoColor=%23F7DF1E) ![HTML5](https://img.shields.io/badge/html5-%23E34F26.svg?style=for-the-badge&logo=html5&logoColor=white) ![CSS3](https://img.shields.io/badge/css3-%231572B6.svg?style=for-the-badge&logo=css3&logoColor=white) ![Vercel](https://img.shields.io/badge/vercel-%23000000.svg?style=for-the-badge&logo=vercel&logoColor=white) ![Telegram](https://img.shields.io/badge/telegram-%232CA5E0.svg?style=for-the-badge&logo=telegram&logoColor=white)

## About the Project

**Time Capsule** is a Telegram Web App (mini app) that lets users create digital time capsules. The project allows you to hide text, a photo, or a voice message inside a 3D safe and set special conditions for opening it. I built this project for my portfolio and to gain experience with the Go language (this is my first project in Golang).

## Screenshots

<!-- Add your screenshots below -->

<p align="center">
  <img src="screenshots/screenshot1.png" alt="Capsule creation screen" width="250"/>
  <img src="screenshots/screenshot2.png" alt="3D safe view" width="250"/>
  <img src="screenshots/screenshot3.png" alt="Opened capsule" width="250"/>
</p>

## How It Works

1. The user opens the mini app via the bot and starts creating a capsule.
2. Chooses a 3D model (safe, heart-shaped box, etc.) and uploads their secret.
3. Sets an opening condition. This can be a timer (from 1 hour to a month), a geolocation (the capsule only opens if the recipient physically arrives at the specified point on the map), or a group collection.
4. Optionally, a PIN code can be added, or "breaking" the safe can be allowed for a certain amount of Telegram Stars.
5. The app generates a link that can be sent to friends on Telegram.
6. Once the conditions are met, the 3D safe opens and reveals the secret. Users can leave emoji reactions.

## Tech Stack

- **Backend:** Golang (using the `gotgbot/v2` library)
- **Database:** MongoDB
- **Frontend:** HTML, CSS, Vanilla JS — no heavy frameworks, for maximum load speed inside Telegram
- **Additional:** Leaflet (maps and geolocation), Model-Viewer (rendering `.glb` 3D models), Canvas Confetti
- **Deployment:** Vercel (Serverless Functions + Static)

## Environment Variables

Only two environment variables are required to run the project (both locally and on Vercel):

| Variable     | Description                                                                 |
|--------------|-------------------------------------------------------------------------------|
| `BOT_TOKEN`  | Your Telegram bot token, obtained from BotFather                             |
| `MONGO_URI`  | Your MongoDB connection string (e.g., a cluster on MongoDB Atlas)            |

## Deployment Instructions (Vercel)

1. Push the project to your GitHub.
2. Create a new project in Vercel and connect the repository.
3. In the **Environment Variables** section, add `BOT_TOKEN` and `MONGO_URI`.
4. Click **Deploy**. Routing is configured automatically via the `vercel.json` file (the Go backend runs as a Serverless function at the `/api` path, while the frontend is served as static content).
5. After a successful deployment, you need to link the bot's webhook to your new domain. To do this, make a GET request in your browser using the following link:

```
   https://api.telegram.org/bot[YOUR_BOT_TOKEN]/setWebhook?url=https://[YOUR_VERCEL_DOMAIN]/api/webhook
```