### [20:05] [config] Parakeet model ID and cache location
**Details**: For Parakeet engine, the model_path config field is NOT a file path - it's a HuggingFace model identifier. The correct value is "mlx-community/parakeet-tdt-0.6b-v3" (not "nvidia/parakeet-tdt-1.1b" which was mentioned in early docs). 

The model is automatically downloaded and cached in ~/.cache/parakeet-mlx/ (NOT ~/.cache/huggingface/) on first use by the parakeet_mlx library's from_pretrained() function. No manual download needed - just specify the model ID in config and it downloads automatically.

Build script (build-mac.sh) installs the parakeet-mlx Python package but doesn't download the model - that happens on first server startup with engine: "parakeet".
**Files**: server/config.example.yaml, scripts/build-mac.sh, scripts/parakeet_worker.py, .claude/knowledge/architecture/parakeet-integration.md
---

