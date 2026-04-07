#!/usr/bin/env sh

ECONUMO_CONFIG_API_URL="${ECONUMO_CONFIG_API_URL:=}"
LILTAG_CONFIG_URL="${LILTAG_CONFIG_URL:=}"
LILTAG_CACHE_TTL="${LILTAG_CACHE_TTL:=0}"
ALLOW_REGISTRATION="${ALLOW_REGISTRATION:=true}"
PAYWALL_ENABLED="${PAYWALL_ENABLED:=false}"
echo "window.econumoConfig = {
  API_URL: \"${ECONUMO_CONFIG_API_URL}\",
  LILTAG_CONFIG_URL: \"${LILTAG_CONFIG_URL}\",
  LILTAG_CACHE_TTL: ${LILTAG_CACHE_TTL},
  ALLOW_REGISTRATION: ${ALLOW_REGISTRATION},
  PAYWALL_ENABLED: ${PAYWALL_ENABLED}
}" > /usr/share/nginx/html/econumo-config.js

chown -R www-data:www-data /var/www/ /usr/share/nginx/html/

if [ ! -d "/var/www/var/db" ]; then
  mkdir -p /var/www/var/db
  chown -R www-data:www-data /var/www/var/db
  su-exec www-data sh -c "cd /var/www && php bin/console doctrine:database:create -q"
fi

su-exec www-data sh -c "cd /var/www && php bin/console doctrine:migrations:migrate --quiet --no-interaction --allow-no-migration"
su-exec www-data sh -c "cd /var/www && php bin/console cache:clear"

/usr/bin/supervisord -n -c /etc/supervisord.conf