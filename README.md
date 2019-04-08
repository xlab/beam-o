# Beam-O [![Powered][1]][2] [![MIT licensed][3]][4]

[1]: https://img.shields.io/badge/KCP-Powered-blue.svg
[2]: https://github.com/xtaci/kcp-go
[3]: https://img.shields.io/badge/license-MIT-blue.svg
[4]: LICENSE.md

### Tips from [kcptun](https://github.com/xtaci/kcptun) project

Increase the number of open files on your server, as:

`ulimit -n 65535`, or write it in `~/.bashrc`.

Suggested `sysctl.conf` parameters for better handling of UDP packets:

```
net.core.rmem_max=26214400 // BDP - bandwidth delay product
net.core.rmem_default=26214400
net.core.wmem_max=26214400
net.core.wmem_default=26214400
net.core.netdev_max_backlog=2048 // proportional to -rcvwnd
```

You can also increase the per-socket buffer by adding parameter(default 4MB):
```
-sockbuf 16777217
```
for **slow processors**, increasing this buffer is **CRITICAL** to receive packets properly.
