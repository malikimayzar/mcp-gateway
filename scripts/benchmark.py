import requests
import time
import statistics
import json
from datetime import datetime

BASE_URL = "http://localhost:8090"

QUERIES = [
    "what is RAG?",
    "explain attention mechanism in transformers",
    "what is BM25 retrieval?",
    "how does FAISS work?",
    "what is faithfulness evaluation?",
]

def bench(name, fn, n=3):
    times = []
    errors = 0
    results = []

    for i in range(n):
        start = time.time()
        try:
            result = fn()
            elapsed = (time.time() - start) * 1000
            times.append(elapsed)
            results.append(result)
        except Exception as e:
            errors += 1
            print(f"  [ERROR] {name} run {i+1}: {e}")

    if not times:
        return {"name": name, "error": "all runs failed"}

    return {
        "name": name,
        "runs": len(times),
        "errors": errors,
        "min_ms": round(min(times), 1),
        "max_ms": round(max(times), 1),
        "avg_ms": round(statistics.mean(times), 1),
        "median_ms": round(statistics.median(times), 1),
        "results": results,
    }

def call_tool(tool_name, params):
    resp = requests.post(f"{BASE_URL}/tool", json={
        "tool_name": tool_name,
        "params": params,
    }, timeout=120)
    resp.raise_for_status()
    data = resp.json()
    return data

def call_plan(query, top_k=3):
    resp = requests.post(f"{BASE_URL}/plan", json={
        "query": query,
        "top_k": top_k,
    }, timeout=300)
    resp.raise_for_status()
    return resp.json()

def call_ask(query, top_k=3):
    resp = requests.post(f"{BASE_URL}/ask", json={
        "query": query,
        "top_k": top_k,
    }, timeout=300)
    resp.raise_for_status()
    return resp.json()

def run_benchmark():
    print("=" * 60)
    print("MCP GATEWAY BENCHMARK")
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"Target: {BASE_URL}")
    print("=" * 60)

    results = {}

    # Health check 
    print("\n[1] Health Check")
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=5)
        data = resp.json()
        tools = data.get("tools", [])
        print(f"  Status : {data.get('status')}")
        print(f"  Tools  : {', '.join(tools)}")
    except Exception as e:
        print(f"  ERROR: {e}")
        return

    # retrieve_chunks 
    print("\n[2] Benchmarking retrieve_chunks (3 runs x 3 queries)")
    retrieve_times = []
    for q in QUERIES[:3]:
        r = bench(f"retrieve_chunks: {q[:30]}", lambda q=q: call_tool("retrieve_chunks", {
            "query": q, "top_k": 5, "method": "hybrid"
        }), n=3)
        retrieve_times.extend([r["min_ms"], r["avg_ms"], r["max_ms"]])
        print(f"  '{q[:35]}' → avg {r['avg_ms']}ms | min {r['min_ms']}ms | max {r['max_ms']}ms")
    results["retrieve_chunks"] = {
        "avg_ms": round(statistics.mean(retrieve_times), 1),
        "min_ms": round(min(retrieve_times), 1),
        "max_ms": round(max(retrieve_times), 1),
    }

    # evaluate_answer 
    print("\n[3] Benchmarking evaluate_answer (3 runs)")
    eval_results = bench("evaluate_answer", lambda: call_tool("evaluate_answer", {
        "question": "what is RAG?",
        "answer": "RAG is Retrieval Augmented Generation, combining retrieval and LLM generation",
        "context": "RAG stands for Retrieval Augmented Generation. It combines a retrieval system with a language model to produce more accurate answers.",
    }), n=3)
    print(f"  avg {eval_results['avg_ms']}ms | min {eval_results['min_ms']}ms | max {eval_results['max_ms']}ms")

    # Extract scores
    scores = []
    for r in eval_results.get("results", []):
        if r and r.get("success") and r.get("data"):
            score = r["data"].get("faithfulness_score", 0)
            scores.append(score)
    if scores:
        print(f"  Faithfulness scores: {scores} → avg {round(statistics.mean(scores), 4)}")
    results["evaluate_answer"] = {
        "avg_ms": eval_results["avg_ms"],
        "min_ms": eval_results["min_ms"],
        "max_ms": eval_results["max_ms"],
        "avg_faithfulness": round(statistics.mean(scores), 4) if scores else None,
    }

    # End-to-end: /plan 
    print("\n[4] Benchmarking /plan end-to-end (2 runs)")
    plan_results = bench("/plan", lambda: call_plan("what is RAG?", top_k=3), n=2)
    print(f"  avg {plan_results['avg_ms']}ms | min {plan_results['min_ms']}ms | max {plan_results['max_ms']}ms")

    plan_scores = []
    for r in plan_results.get("results", []):
        if r and r.get("Score", 0) > 0:
            plan_scores.append(r["Score"])
    if plan_scores:
        print(f"  Eval scores: {plan_scores} → avg {round(statistics.mean(plan_scores), 4)}")
    results["plan"] = {
        "avg_ms": plan_results["avg_ms"],
        "min_ms": plan_results["min_ms"],
        "max_ms": plan_results["max_ms"],
        "avg_score": round(statistics.mean(plan_scores), 4) if plan_scores else None,
    }

    # End-to-end: /ask (Groq) 
    print("\n[5] Benchmarking /ask with Groq orchestrator (2 runs)")
    ask_results = bench("/ask", lambda: call_ask("what is attention mechanism?", top_k=3), n=2)
    print(f"  avg {ask_results['avg_ms']}ms | min {ask_results['min_ms']}ms | max {ask_results['max_ms']}ms")

    ask_scores = []
    orchestrators = []
    for r in ask_results.get("results", []):
        if r:
            if r.get("Score", 0) > 0:
                ask_scores.append(r["Score"])
            if r.get("orchestrator"):
                orchestrators.append(r["orchestrator"])
    if ask_scores:
        print(f"  Eval scores: {ask_scores} → avg {round(statistics.mean(ask_scores), 4)}")
    if orchestrators:
        print(f"  Orchestrators used: {set(orchestrators)}")
    results["ask"] = {
        "avg_ms": ask_results["avg_ms"],
        "min_ms": ask_results["min_ms"],
        "max_ms": ask_results["max_ms"],
        "avg_score": round(statistics.mean(ask_scores), 4) if ask_scores else None,
        "orchestrators": list(set(orchestrators)),
    }

    # Summary 
    print("\n" + "=" * 60)
    print("BENCHMARK SUMMARY")
    print("=" * 60)
    print(f"{'Tool/Endpoint':<25} {'Avg (ms)':>10} {'Min (ms)':>10} {'Max (ms)':>10} {'Score':>8}")
    print("-" * 65)

    for key, val in results.items():
        score = val.get("avg_score") or val.get("avg_faithfulness")
        score_str = f"{score:.4f}" if score else "  -"
        print(f"{key:<25} {val['avg_ms']:>10} {val['min_ms']:>10} {val['max_ms']:>10} {score_str:>8}")

    print("=" * 60)

    # Save hasil ke JSON
    output = {
        "timestamp": datetime.now().isoformat(),
        "gateway": BASE_URL,
        "results": results,
    }
    with open("/tmp/benchmark_results.json", "w") as f:
        json.dump(output, f, indent=2)
    print(f"\nResults saved to /tmp/benchmark_results.json")

if __name__ == "__main__":
    run_benchmark()