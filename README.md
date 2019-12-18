# tcp-over-http

This program is just a proxy server which multiplexes TCP connections into an HTTPS. The primary purpose is to make all connections look like legitimate ones for firewalls (including DPI).

The closest analog is shadowsocks.

The only reason why I wrote this is that I want to have better control on how the connections are masked/multiplexed.

#### Current status

This software is unstable and non-unique. The code is broken in many places and follows the worst coding practices. It is not recommended to use this piece.

### Setup

First, clone the repo and build all binaries (`go install ./...`). You may install it with `go get github.com/neex/tcp-over-http` as well, but the idea that you won't need source modifications is way too optimistic.

#### Server (do this before your trip)

1. Buy a domain name (e.g. `example.com`).
2. Rent a VPS. Make sure not to use any popular VPS provider (e.g. DigitalOcean/Vultr/Scaleway) as they're banned in some countries almost completely.
3. Set up DNS A record pointing to the VPS.
4. Get a LetsEncrypt certificate using certbot (`apt install certbot && certbot -d example.com`).
5. Get something legitimate-looking to place on your website, like free bootstrap template.
6. Create a config for the server part in `/etc/tcp-over-http.yaml`, it should look like this:
   ```yaml
   listen_addr: ':443'
   static_dir: /var/www/html/
   dial_timeout: 2m
   token: <put a random token here>
   domain: <example.com>
   redirector_addr: ':80'
   cert_path: /etc/letsencrypt/live/<example.com>/fullchain.pem
   key_path: /etc/letsencrypt/live/<example.com>/privkey.pem
   ```
7. Create systemd module in `/etc/systemd/system/tcp-over-http.service`:
   ```yaml
   [Unit]
   Description=TCP over HTTP

   [Service]
   Type=simple
   Restart=always
   ExecStart=/usr/local/bin/tcp_over_http_server /etc/tcp-over-http.yaml
   LimitNOFILE=100000

   [Install]
   WantedBy=multi-user.target
   ```
8. Do `systemctl daemon-reload && systemctl start tcp-over-http`.
9. Debug errors in journalctl, if any.

#### Client (test this before your trip)

1. Create a client config, should like like:

   ```yaml
   address: "https://<example.com>/establish/<token-from-server-config>"
   dns_override: <vps ip>:443
   remote_timeout: 30s
   connect_timeout: 10s
   max_connection_multiplex: 1000
   keep_alive_timeout: 10s
   ```
2. Start the client using something like
   ```bash
   tcp_over_http --config ./client.yaml proxy :12321 --direct-dial '127.0.0.1|localhost'
   ```

   This command starts socks5 server on :12321, which runs through the tunnel.

3. Under linux, you can setup an interface that proxies the connections. Do it like this:
   ```bash
   sudo ip tuntap add user <your username> mode tun hui0
   sudo ip link set hui0 up
   ```

   After that, run `tcp-over-http` with `--tun hui0` flag. No server reconfiguration is required.

   Note that this is not an actual VPN. The connections are intercepted and proxies as TCP/UDP streams.

   Also, you will need to set up routes correctly.

### License

This software is distributed under the terms of [MIT License](LICENSE.md).
