<p align="center">
  <img src="keirouter-logo.png" alt="KeiRouter Logo" width="160">
</p>

<h1 align="center">KeiRouter 🚀</h1>

<p align="center">
  <strong>Your friendly, blazing-fast, self-hostable AI gateway.</strong>
</p>

<p align="center">
  <a href="https://github.com/mydisha/keirouter/actions/workflows/ci.yml"><img src="https://github.com/mydisha/keirouter/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

<p align="center">
  <img src="assets/keirouter-social-banner.png" alt="KeiRouter Dashboard" width="800">
</p>

---

Hey there! 👋 Welcome to **KeiRouter**.

If you're using AI coding tools like Claude Code, Cursor, Cline, or literally any app that talks to OpenAI or Anthropic, you know the struggle: juggling API keys, hitting rate limits, and watching your token costs explode.

**KeiRouter fixes that.**

You just point all your AI tools to one local endpoint on your machine. KeiRouter acts as your smart middleman—routing requests to the right AI models, automatically falling back if a service goes down, caching repeated questions to save money, and compressing big chunks of text before they even reach the AI!

It's built with Go, which means it’s incredibly lightweight (using barely ~20MB of RAM) and starts up instantly. Plus, it comes with a beautiful dashboard to manage everything. ✨

> **Status:** Active development. See the [Architecture](#-architecture-for-the-curious) section for what's implemented.

## 🌟 Why you'll love KeiRouter

- 🔀 **One Endpoint to Rule Them All:** Your apps only need to speak OpenAI or Anthropic. KeiRouter handles translating your requests to whatever provider you actually want to use behind the scenes.
- 🛡️ **Never Stop Coding:** Hit a rate limit? Provider down? KeiRouter automatically falls back to your backup models so your workflow never gets interrupted.
- 💸 **Save Serious Cash:** 
  - **Input Compression:** It shrinks massive logs, code diffs, and file structures before sending them to the LLM. 
  - **Output Compression:** Tell the AI to speak in "terse mode" to cut out all the yapping and just give you the code.
- 🔐 **Super Secure:** Your API keys are encrypted with military-grade envelope encryption (AES-256-GCM). We never store plain text keys. Your local dashboard is also protected by a secure password and HMAC session cookies.
- 📊 **Track Everything:** Wondering where your money is going? The dashboard gives you beautiful charts showing your token usage, costs, and latency.
- ⚡ **Lightning Fast Caching:** Ask the same question twice? The semantic cache remembers the answer and gives it back to you instantly, for exactly $0.00.

## 🚀 Let's get started!

### 1. Install KeiRouter
You can install it in a snap using our quick script (make sure you have Go 1.22+ and Node.js 20+):
```bash
curl -fsSL https://raw.githubusercontent.com/mydisha/keirouter/main/scripts/install.sh | bash
```

*Prefer Docker? We've got you covered:*
```bash
docker compose -f deploy/compose.yaml up -d --build
```

### 2. Set up your Dashboard
Once it's running, open your browser and go to **http://localhost:20180**. 
Log in with the default password: `keirouter` (it will ask you to change this immediately for security).

You can also create an API key directly from the terminal without the dashboard:
```bash
keirouter -bootstrap   # prints a kr_ key once
```

### 3. Connect your tools
In your favorite AI tool (like Cursor or Claude Code), set it up like this:
- **Base URL:** `http://localhost:20180/v1`
- **API Key:** Use the `kr_` key you generated in the dashboard or via bootstrap
- **Model:** Type the provider and model (e.g., `openai/gpt-4o`) or the name of a fallback chain you created!

## 🧠 Smart Routing (Chains)
Instead of just picking one model, you can create a "Chain" in the dashboard. 

For example, a chain named `coding` could try:
1. `openai/gpt-4o` (First choice)
2. `deepseek/deepseek-chat` (If GPT-4o fails or hits a rate limit)

Then, in your app, just set the model to `chain:coding` (or just `coding`) and let KeiRouter do the heavy lifting!

## 🔌 What else can it do?
KeiRouter isn't just for chat! It supports everything:
- **Image Generation** (`/v1/images/generations`)
- **Speech-to-Text** (`/v1/audio/transcriptions`)
- **Text-to-Speech** (`/v1/audio/speech`)
- **Web Search & Fetching** (`/v1/search`, `/v1/web/fetch`)
- **Embeddings** (`/v1/embeddings`)

## 🔑 Connect via OAuth (No API Keys needed!)
Tired of copying API keys? You can connect providers like Claude, GitHub Copilot, Gemini CLI, and more directly from the Connections page using OAuth. Just click, sign in, and KeiRouter handles securely refreshing your tokens in the background!

## 🛠️ Architecture for the curious
Curious how it works under the hood? Here's the life of a request:
1. **Gateway:** Receives your HTTP request and parses the AI dialect (OpenAI, Anthropic, Gemini, etc.).
2. **Pipeline:** Compresses your inputs (Slimmer), injects cost-saving prompts (Terse mode), and checks your budget limits.
3. **Dispatch:** Picks the best provider account and handles fallbacks.
4. **Connector & Transform:** Talks to the provider and translates the response back into the format your tool expects.
5. **Meter:** Logs how many tokens you used so you can view it on the dashboard.

## ⚙️ Configuration
By default, KeiRouter uses an embedded SQLite database (zero config required!). If you are deploying it for a team, you can use PostgreSQL. Just copy `config.example.yaml` and run with `-config`, or use environment variables like `KEIROUTER_SERVER__PORT=8080`.

## 🔒 Security Notes
- The admin API (`/api/*`) is restricted to your local machine by default. If you expose it to the internet, put it behind a reverse proxy and set a stable `master_key`!
- **Don't lose your master key!** It is the root of trust for all your encrypted credentials.

## 🧑‍💻 Development & Contributing
Want to hack on KeiRouter?
```bash
make dev   # Runs the backend on :20180 and dashboard on :5180
```
Contributions are always welcome! Check out [CONTRIBUTING.md](CONTRIBUTING.md) to see how you can get involved.

## 🛡️ Security Vulnerabilities
If you find a security issue, please check [SECURITY.md](SECURITY.md) instead of opening a public issue.

## 📄 License
MIT License - See [LICENSE](LICENSE) for details.