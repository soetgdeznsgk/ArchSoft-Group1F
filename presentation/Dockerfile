FROM nginx:1.30.5-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf

COPY libraries/ /usr/share/nginx/html/libraries/
COPY lobby/ /usr/share/nginx/html/menu/
COPY game/ /usr/share/nginx/html/ingame/
COPY chameleon/ /usr/share/nginx/html/chameleon/
