# Telegram bot as Transmission RPC interface
An interface bot that helps managing torrents on a local machine.

Currently only implements https://rutracker.org's types and html layout.

## Environmental variables
1. FORUM_URL: torrent tracker http endpoint, must be `https://rutracker.org/forum`
2. BB_SESSION: value of the `bb_session` cookie. You must log in to rutracker in order to get the value. Note that this session cookies are only valid for 1 year.
3. TELEGRAM_BOT_API_TOKEN: secure token for your telegram bot, obtained through BotFather.

## Deployment
A typical deployment is based on docker-compose, with both transmission server and bot running in the same composition. Typically:

```yaml
version: "2.1"
services:
  transmission:
    image: lscr.io/linuxserver/transmission:latest
    container_name: transmission
    environment:
      - PUID=1000
      - PGID=1000
    volumes:
      - ./config:/config
      - ./watch:/watch
      - /media/viceversa/Downloads:/downloads/complete
      - /media/viceversa/Incomplete:/downloads/incomplete
    ports:
      - 9091:9091
      - 51413:51413
      - 51413:51413/udp
    restart: unless-stopped

  bot:
    image: arkhipovkm/transmission-bot
    restart: always
    environment:
      FORUM_URL: https://rutracker.org/forum
      TRANSMISSION_RPC_HOST: transmission
      BB_SESSION: <your bb-session cookie value>
      TELEGRAM_BOT_API_TOKEN: <your telegram bot api token>
```