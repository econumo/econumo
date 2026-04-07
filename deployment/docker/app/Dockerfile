# Backend development stage
FROM php:8.5-fpm-alpine AS dev

EXPOSE 80 9000

ENV PHP_DISPLAY_ERRORS=0
ENV PHP_ERROR_REPORTING=22527
ENV COMPOSER_ALLOW_SUPERUSER=1

WORKDIR /var/www/
RUN apk add --update nginx supervisor git busybox-suid postgresql-dev

RUN apk add --no-cache --virtual .build-deps $PHPIZE_DEPS curl-dev && \
    docker-php-ext-configure bcmath --enable-bcmath && \
    docker-php-ext-configure pgsql --with-pgsql=/usr/local/pgsql && \
    docker-php-ext-install -j$(nproc) bcmath curl pdo_pgsql pgsql && \
    apk del -f .build-deps && \
    rm -rf /tmp/* /var/cache/apk/*

RUN mv "$PHP_INI_DIR/php.ini-development" "$PHP_INI_DIR/php.ini"

# Copy custom configs
COPY deployment/docker/app/configs /

# composer
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer

CMD ["/usr/bin/supervisord", "-n", "-c", "/etc/supervisord.conf"]


FROM dev AS test

COPY composer.json composer.lock symfony.lock /var/www/
RUN composer install --no-scripts --prefer-dist --no-autoloader
COPY . /var/www
# Finish composer
RUN composer dump-autoload --optimize
# Install assets
RUN bin/console assets:install public && \
    bin/console nelmio:apidoc:dump
RUN chown www-data:www-data /var/www/ -R


# Frontend build stage (only for production)
FROM node:25-alpine AS frontend-builder

WORKDIR /build/web

RUN npm install -g pnpm@10

# Copy frontend files
COPY web/package.json web/pnpm-lock.yaml* ./
RUN pnpm install --shamefully-hoist

COPY web/ ./
RUN pnpm run build


FROM test AS prod

EXPOSE 80

RUN mv "$PHP_INI_DIR/php.ini-production" "$PHP_INI_DIR/php.ini"

# Install su-exec for Alpine-compatible user switching in entrypoint
RUN apk add --no-cache su-exec

# Remove dev dependencies from vendor
RUN composer install --no-dev --no-scripts --optimize-autoloader --classmap-authoritative && \
    composer clear-cache

# Warmup cache
RUN rm -rf /var/www/var/cache/* && bin/console cache:warmup --env=prod
RUN chown www-data:www-data /var/www/var/ -R

# Copy frontend dist from frontend-builder stage
COPY --from=frontend-builder /build/web/dist/spa /usr/share/nginx/html

# Cleanup dev/test dependencies
RUN rm -rf /var/www/.env.* && \
    rm -rf /var/www/tests && \
    rm -rf /var/www/web && \
    rm -rf /var/www/codeception.yml && \
    rm -rf /var/www/rector.php && \
    rm -rf /var/www/psalm.xml && \
    rm -rf /var/www/.github && \
    rm -f /usr/local/bin/composer && \
    apk del git busybox-suid && \
    rm -rf /var/cache/apk/* /tmp/*

RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
