# BUG: homelab.otthol.hu public DNS :443 hits hub Traefik, not HA VIP

- **Stage:** B7 / B7b
- **Symptom:** `curl -4k https://whoami.homelab.otthol.hu/` → hub Traefik DEFAULT CERT + HTTP 404. Site whoami is healthy on VIP `172.16.10.100:30443` and WG upstream `10.10.0.12:30443` (HTTP 200 whoami body).
- **DNS:** `whoami.homelab.otthol.hu` / `*.homelab.otthol.hu` → `84.1.215.254` (ottholhu.ddns.net). Hub listens on `*:443`; WAN forward appears to land on hub, not `homelab-ha-pf` VIP NodePort 30443.
- **Impact:** Public URL checks fail; Let's Encrypt HTTP-01 for spoke site hosts cannot complete (ACME stays PendingStaging).
- **Workaround used:** Verify site routes with `curl -4 -sk --resolve <host>:30443:10.10.0.12` from hub, or jumphost → `https://172.16.10.100:30443` + Host header.
- **Expected:** Router PF WAN:80→VIP:30080 and WAN:443→VIP:30443 for homelab zone (or split DNS), so public names reach spoke Traefik.
- **Assets:** `B7-whoami-site-curl.txt`
