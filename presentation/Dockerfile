FROM nginx:1.30.5-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf

COPY libraries/ /usr/share/nginx/html/libraries/
COPY menu/ /usr/share/nginx/html/menu/
COPY ingame/ /usr/share/nginx/html/ingame/

