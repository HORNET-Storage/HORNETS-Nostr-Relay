package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type setupPayload struct {
	RelayConfig       map[string]interface{} `json:"relayConfig"`
	AirlockConfig     map[string]interface{} `json:"airlockConfig"`
	AirlockConfigPath string                 `json:"airlockConfigPath"`
}

func setupMarkerPath() string {
	dataPath := viper.GetString("server.data_path")
	if dataPath == "" {
		dataPath = "./data"
	}
	return filepath.Join(dataPath, ".hornets_setup_complete")
}

func setupToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeYAMLAtomic(path string, data map[string]interface{}) error {
	if len(data) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, encoded, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeSetupMarker() error {
	marker := setupMarkerPath()
	if err := os.MkdirAll(filepath.Dir(marker), os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0644)
}

func needsBootstrapSetup() bool {
	_, err := os.Stat(setupMarkerPath())
	return errors.Is(err, os.ErrNotExist)
}

func defaultAirlockConfigPath() string {
	path := os.Getenv("AIRLOCK_CONFIG_PATH")
	if path == "" {
		path = filepath.Join("..", "airlock", "config.yaml")
	}
	return path
}

func readYAMLMap(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}
	}
	parsed := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return map[string]interface{}{}
	}
	if parsed == nil {
		return map[string]interface{}{}
	}
	return parsed
}

func runBootstrapSetup(ctx context.Context, host string, port int) error {
	if !needsBootstrapSetup() {
		logging.Info("Bootstrap setup already completed - continuing normal startup", nil)
		return nil
	}

	token, err := setupToken()
	if err != nil {
		return fmt.Errorf("failed to generate bootstrap token: %w", err)
	}

	applyCh := make(chan struct{}, 1)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	mustAuth := func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodGet {
			return c.Next()
		}
		if c.Get("X-Setup-Token") != token {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid setup token"})
		}
		return c.Next()
	}

	app.Use(mustAuth)

	app.Get("/setup/defaults", func(c *fiber.Ctx) error {
		relayDefaults := readYAMLMap("config.example.yaml")
		airlockDefaults := readYAMLMap(filepath.Join("..", "airlock", "config.example.yaml"))
		return c.JSON(fiber.Map{
			"relayConfig":       relayDefaults,
			"airlockConfig":     airlockDefaults,
			"airlockConfigPath": defaultAirlockConfigPath(),
		})
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return c.Type("html").SendString(fmt.Sprintf(`<!doctype html>
<html>
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>HORNETS First-Time Setup</title>
	<style>
		:root {
			--bg: #0b1020;
			--card: #11182a;
			--border: #22304b;
			--text: #eaf0ff;
			--muted: #93a4c4;
			--accent: #39c48e;
			--accent2: #4f7cff;
			--danger: #ff6b6b;
		}
		* { box-sizing: border-box; }
		body {
			margin: 0;
			font-family: "Segoe UI", "Inter", sans-serif;
			background: radial-gradient(1200px 500px at 10%% -20%%, #1f2f56, transparent), var(--bg);
			color: var(--text);
		}
		.wrap { max-width: 1100px; margin: 0 auto; padding: 24px; }
		.hero { margin-bottom: 18px; }
		.hero h1 { margin: 0; font-size: 30px; letter-spacing: .2px; }
		.hero p { color: var(--muted); margin-top: 8px; }
		.grid { display: grid; gap: 14px; grid-template-columns: 1fr; }
		@media (min-width: 920px) { .grid { grid-template-columns: 1fr 1fr; } }
		.card {
			background: linear-gradient(180deg, rgba(255,255,255,.015), rgba(255,255,255,.005));
			border: 1px solid var(--border);
			border-radius: 14px;
			padding: 16px;
		}
		.card h2 { margin: 0 0 8px; font-size: 18px; }
		.hint { color: var(--muted); font-size: 13px; margin-bottom: 12px; }
		.row { display: grid; grid-template-columns: 1fr; gap: 8px; margin-bottom: 10px; }
		.row.two { grid-template-columns: 1fr 1fr; }
		label { font-size: 12px; color: var(--muted); }
		input, textarea {
			width: 100%%;
			border: 1px solid var(--border);
			background: #0d1425;
			color: var(--text);
			border-radius: 8px;
			padding: 10px 12px;
			outline: none;
			font-size: 14px;
		}
		textarea { min-height: 110px; font-family: Consolas, monospace; }
		.actions { display: flex; gap: 10px; margin-top: 14px; flex-wrap: wrap; }
		button {
			border: 0;
			border-radius: 8px;
			padding: 10px 14px;
			font-weight: 600;
			cursor: pointer;
		}
		.primary { background: var(--accent); color: #042e1f; }
		.secondary { background: #243455; color: #e6ecff; }
		.ghost { background: #17213a; color: #dce6ff; }
		.status {
			margin-top: 14px;
			padding: 10px 12px;
			border-radius: 8px;
			border: 1px solid var(--border);
			background: #0c1528;
			white-space: pre-wrap;
			font-size: 13px;
			color: #d4ddf5;
		}
		.ok { border-color: #2f8f6f; }
		.bad { border-color: #8f3e3e; color: #ffd5d5; }
		details { margin-top: 8px; }
		summary { cursor: pointer; color: #b9c8e8; }
	</style>
</head>
<body>
	<div class="wrap">
		<div class="hero">
			<h1>Welcome to HORNETS</h1>
			<p>Let's configure your Relay and Airlock in a guided first-time setup.</p>
		</div>

		<div class="grid">
			<section class="card">
				<h2>Relay Setup</h2>
				<div class="hint">Identity and service details that nestr clients will discover.</div>
				<div class="row two">
					<div><label>Name</label><input id="relay_name" placeholder="HORNETS"></div>
					<div><label>Contact Email</label><input id="relay_contact" placeholder="support@example.com"></div>
				</div>
				<div class="row">
					<div><label>Description</label><input id="relay_description" placeholder="Your relay description"></div>
				</div>
				<div class="row two">
					<div><label>Icon URL</label><input id="relay_icon" placeholder="https://.../logo.png"></div>
					<div><label>Service Tag</label><input id="relay_service_tag" placeholder="hornet-storage-service"></div>
				</div>
				<div class="row two">
					<div><label>Relay Private Key (hex)</label><input id="relay_private_key"></div>
					<div><label>Relay Public Key (hex)</label><input id="relay_public_key"></div>
				</div>
				<div class="row two">
					<div><label>DHT Key</label><input id="relay_dht_key"></div>
					<div><label>Secret Key (shared)</label><input id="relay_secret_key"></div>
				</div>
				<div class="actions">
					<button class="secondary" type="button" onclick="generateRelayKeys()">Generate Relay Keys</button>
					<button class="ghost" type="button" onclick="generateSecret()">Generate Secret</button>
				</div>
			</section>

			<section class="card">
				<h2>Airlock Setup</h2>
				<div class="hint">Repository sidecar and relay connectivity used by Airlock.</div>
				<div class="row two">
					<div><label>Bind Address</label><input id="airlock_bind_address" placeholder="0.0.0.0"></div>
					<div><label>Port</label><input id="airlock_port" type="number" placeholder="11006"></div>
				</div>
				<div class="row two">
					<div><label>Relay Address (host:port)</label><input id="airlock_relay" placeholder="127.0.0.1:11000"></div>
					<div><label>Repository Path</label><input id="airlock_repository_path" placeholder="repositories"></div>
				</div>
				<div class="row">
					<div><label>Airlock Private Key (nsec or hex)</label><input id="airlock_private_key"></div>
				</div>
				<div class="row two">
					<div><label>Sidecar Address</label><input id="airlock_sidecar_address" placeholder="127.0.0.1:9100"></div>
					<div><label>Sidecar Mode</label><input id="airlock_sidecar_mode" placeholder="persistent"></div>
				</div>
				<div class="row">
					<div><label>Airlock Config Path</label><input id="airlock_config_path"></div>
				</div>
				<div class="actions">
					<button class="ghost" type="button" onclick="generateAirlockKey()">Generate Airlock Key</button>
				</div>
			</section>
		</div>

		<section class="card" style="margin-top:14px;">
			<h2>Advanced Overrides (optional)</h2>
			<div class="hint">Only for advanced users. JSON here merges onto generated Relay/Airlock config.</div>
			<div class="row two">
				<div>
					<label>Relay JSON Overrides</label>
					<textarea id="relay_overrides" placeholder='{"logging":{"level":"debug"}}'></textarea>
				</div>
				<div>
					<label>Airlock JSON Overrides</label>
					<textarea id="airlock_overrides" placeholder='{"sidecar":{"executable":"/usr/local/bin/hornets-hyperswarm"}}'></textarea>
				</div>
			</div>
			<details>
				<summary>Preview full payload JSON</summary>
				<textarea id="payload_preview"></textarea>
			</details>
			<div class="actions">
				<button class="secondary" type="button" onclick="validateSetup()">Validate</button>
				<button class="primary" type="button" onclick="applySetup()">Apply Setup</button>
			</div>
			<div id="status" class="status">Waiting for input...</div>
		</section>
	</div>

	<script>
		const token = %q;
		let defaults = { relayConfig: {}, airlockConfig: {}, airlockConfigPath: "" };

		function randomHex(bytes) {
			const a = new Uint8Array(bytes);
			crypto.getRandomValues(a);
			return Array.from(a).map(v => v.toString(16).padStart(2, "0")).join("");
		}

		function el(id) { return document.getElementById(id); }

		function setStatus(msg, ok=true) {
			const s = el("status");
			s.textContent = msg;
			s.className = ok ? "status ok" : "status bad";
		}

		function deepMerge(target, src) {
			for (const [k, v] of Object.entries(src || {})) {
				if (v && typeof v === "object" && !Array.isArray(v)) {
					target[k] = deepMerge(target[k] && typeof target[k] === "object" ? target[k] : {}, v);
				} else {
					target[k] = v;
				}
			}
			return target;
		}

		function parseOverride(id) {
			const t = el(id).value.trim();
			if (!t) return {};
			try {
				const parsed = JSON.parse(t);
				if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
					throw new Error("must be a JSON object");
				}
				return parsed;
			} catch (err) {
				throw new Error(id + ": " + err.message);
			}
		}

		function buildPayload() {
			const relay = JSON.parse(JSON.stringify(defaults.relayConfig || {}));
			const airlock = JSON.parse(JSON.stringify(defaults.airlockConfig || {}));

			relay.relay = relay.relay || {};
			relay.relay.name = el("relay_name").value.trim();
			relay.relay.contact = el("relay_contact").value.trim();
			relay.relay.description = el("relay_description").value.trim();
			relay.relay.icon = el("relay_icon").value.trim();
			relay.relay.service_tag = el("relay_service_tag").value.trim();
			relay.relay.private_key = el("relay_private_key").value.trim();
			relay.relay.public_key = el("relay_public_key").value.trim();
			relay.relay.dht_key = el("relay_dht_key").value.trim();
			relay.relay.secret_key = el("relay_secret_key").value.trim();

			airlock.bind_address = el("airlock_bind_address").value.trim();
			airlock.port = Number(el("airlock_port").value || "0");
			airlock.relay = el("airlock_relay").value.trim();
			airlock.repository_path = el("airlock_repository_path").value.trim();
			airlock.private_key = el("airlock_private_key").value.trim();
			airlock.sidecar = airlock.sidecar || {};
			airlock.sidecar.address = el("airlock_sidecar_address").value.trim();
			airlock.sidecar.mode = el("airlock_sidecar_mode").value.trim();

			deepMerge(relay, parseOverride("relay_overrides"));
			deepMerge(airlock, parseOverride("airlock_overrides"));

			const payload = {
				relayConfig: relay,
				airlockConfig: airlock,
				airlockConfigPath: el("airlock_config_path").value.trim() || defaults.airlockConfigPath || ""
			};

			el("payload_preview").value = JSON.stringify(payload, null, 2);
			return payload;
		}

		async function request(path, method, body) {
			const res = await fetch(path, {
				method,
				headers: {
					"Content-Type": "application/json",
					"X-Setup-Token": token
				},
				body: body ? JSON.stringify(body) : undefined
			});
			const data = await res.json().catch(() => ({}));
			return {res, data};
		}

		async function loadDefaults() {
			const res = await fetch("/setup/defaults", { cache: "no-store" });
			defaults = await res.json();

			const relay = defaults.relayConfig?.relay || {};
			el("relay_name").value = relay.name || "";
			el("relay_contact").value = relay.contact || "";
			el("relay_description").value = relay.description || "";
			el("relay_icon").value = relay.icon || "";
			el("relay_service_tag").value = relay.service_tag || "";
			el("relay_private_key").value = relay.private_key || "";
			el("relay_public_key").value = relay.public_key || "";
			el("relay_dht_key").value = relay.dht_key || "";
			el("relay_secret_key").value = relay.secret_key || "";

			const airlock = defaults.airlockConfig || {};
			el("airlock_bind_address").value = airlock.bind_address || "";
			el("airlock_port").value = String(airlock.port || "");
			el("airlock_relay").value = airlock.relay || "";
			el("airlock_repository_path").value = airlock.repository_path || "";
			el("airlock_private_key").value = airlock.private_key || "";
			el("airlock_sidecar_address").value = airlock.sidecar?.address || "";
			el("airlock_sidecar_mode").value = airlock.sidecar?.mode || "persistent";
			el("airlock_config_path").value = defaults.airlockConfigPath || "";

			buildPayload();
			setStatus("Defaults loaded. Configure values and click Validate or Apply.");
		}

		function generateRelayKeys() {
			el("relay_private_key").value = randomHex(32);
			el("relay_public_key").value = randomHex(32);
			el("relay_dht_key").value = randomHex(20);
			buildPayload();
			setStatus("Generated relay keys. Replace with your own if needed.");
		}

		function generateSecret() {
			el("relay_secret_key").value = randomHex(32);
			buildPayload();
			setStatus("Generated relay secret key.");
		}

		function generateAirlockKey() {
			el("airlock_private_key").value = randomHex(32);
			buildPayload();
			setStatus("Generated airlock private key. If you use nsec format, paste it manually.");
		}

		async function validateSetup() {
			try {
				const payload = buildPayload();
				const {res, data} = await request('/setup/validate', 'POST', payload);
				setStatus(JSON.stringify({ status: res.status, result: data }, null, 2), res.ok);
			} catch (err) {
				setStatus(String(err), false);
			}
		}

		async function applySetup() {
			try {
				const payload = buildPayload();
				const {res, data} = await request('/setup/apply', 'POST', payload);
				setStatus(JSON.stringify({ status: res.status, result: data }, null, 2), res.ok);
				if (res.ok) {
					setTimeout(() => {
						setStatus("Setup applied. Relay is transitioning to normal startup...", true);
					}, 500);
				}
			} catch (err) {
				setStatus(String(err), false);
			}
		}

		["input", "change"].forEach(evt => {
			document.addEventListener(evt, () => {
				try { buildPayload(); } catch {}
			});
		});

		loadDefaults().catch(err => setStatus("Failed loading defaults: " + err, false));
	</script>
</body>
</html>`, token))
	})

	app.Get("/setup", func(c *fiber.Ctx) error {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	})

	app.Get("/setup/status", func(c *fiber.Ctx) error {
		_, relayCfgErr := os.Stat("config.yaml")
		airlockPath := defaultAirlockConfigPath()
		_, airlockCfgErr := os.Stat(airlockPath)
		return c.JSON(fiber.Map{
			"needs_setup":           needsBootstrapSetup(),
			"bootstrap_complete":    !needsBootstrapSetup(),
			"relay_config_exists":   relayCfgErr == nil,
			"airlock_config_exists": airlockCfgErr == nil,
		})
	})

	app.Post("/setup/validate", func(c *fiber.Ctx) error {
		var payload setupPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
		}
		if payload.RelayConfig == nil {
			payload.RelayConfig = map[string]interface{}{}
		}
		if payload.AirlockConfig == nil {
			payload.AirlockConfig = map[string]interface{}{}
		}
		relayCfg, _ := payload.RelayConfig["relay"].(map[string]interface{})
		priv := strings.TrimSpace(fmt.Sprint(relayCfg["private_key"]))
		if priv == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": "relay.private_key is required"})
		}

		return c.JSON(fiber.Map{
			"ok":                  true,
			"relay_config_keys":   len(payload.RelayConfig),
			"airlock_config_keys": len(payload.AirlockConfig),
		})
	})

	app.Post("/setup/apply", func(c *fiber.Ctx) error {
		var payload setupPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid payload"})
		}

		if payload.RelayConfig != nil && len(payload.RelayConfig) > 0 {
			if err := writeYAMLAtomic("config.yaml", payload.RelayConfig); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
		}

		if payload.AirlockConfig != nil && len(payload.AirlockConfig) > 0 {
			path := payload.AirlockConfigPath
			if path == "" {
				path = defaultAirlockConfigPath()
			}
			if err := writeYAMLAtomic(path, payload.AirlockConfig); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
			}
		}

		if err := writeSetupMarker(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		select {
		case applyCh <- struct{}{}:
		default:
		}

		return c.JSON(fiber.Map{"ok": true, "message": "setup saved"})
	})

	app.Post("/setup/abort", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	addr := fmt.Sprintf("%s:%d", host, port)
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Listen(addr)
	}()

	logging.Info("Bootstrap setup mode enabled - waiting for setup completion", map[string]interface{}{
		"addr": addr,
	})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
		_ = app.Shutdown()
		return ctx.Err()
	case <-sigCh:
		_ = app.Shutdown()
		return fmt.Errorf("bootstrap setup interrupted")
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-applyCh:
		_ = app.Shutdown()
		logging.Info("Bootstrap setup completed - continuing normal startup", nil)
		return nil
	}
}
