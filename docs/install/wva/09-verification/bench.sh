 pip install aiohttp transformers

# Download the benchmark script and dataset
curl -LO https://raw.githubusercontent.com/vllm-project/vllm/main/benchmarks/benchmark_serving.py
curl -LO https://huggingface.co/datasets/anon8231489123/ShareGPT_Vicuna_unfiltered/resolve/main/ShareGPT_V3_unfil
tered_cleaned_split.json

# Run against your port-forward (start port-forward first)
python3 benchmark_serving.py \
--backend openai-chat \
--base-url https://localhost:9443/<namespace>/<isvc-name> \
--model Qwen/Qwen2.5-7B-Instruct \
--dataset-name sharegpt \
--dataset-path ShareGPT_V3_unfiltered_cleaned_split.json \
--num-prompts 1000 \
--request-rate 100 \
--max-concurrency 200
