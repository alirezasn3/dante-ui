#! /bin/bash

curl -O https://github.com/alirezasn3/dante-ui/releases/download/v1.0.0/dante-ui-linux-amd64
mv dante-ui-linux-amd64 dante-ui
chmod +x dante-ui
cp dante-ui.service /etc/systemd/system/
systemctl enable dante-ui
systemctl start dante-ui
systemctl status dante-ui