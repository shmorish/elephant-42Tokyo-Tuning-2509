#!/bin/bash

# シンボリックリンクを削除
rm -f /var/log/nginx/access.log /var/log/nginx/error.log

# 実際のログファイルを作成
touch /var/log/nginx/access.log /var/log/nginx/error.log

# ファイルの権限を設定
chown nginx:nginx /var/log/nginx/access.log /var/log/nginx/error.log
chmod 644 /var/log/nginx/access.log /var/log/nginx/error.log

# alpのインストール
apt update
apt install -y wget tar
wget https://github.com/tkuchiki/alp/releases/download/v1.0.21/alp_linux_amd64.tar.gz
tar zxvf alp_linux_amd64.tar.gz
install alp /usr/local/bin

# nginxを起動
exec "$@"