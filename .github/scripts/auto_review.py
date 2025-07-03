import os
import sys
import json
import argparse
from github import Github
from openai import OpenAI

parser = argparse.ArgumentParser()
parser.add_argument("--pr", type=int, help="PR number to review")
args = parser.parse_args()

pr_number = args.pr

# ─── Load webhook/event payload ────────────────────────────────
event_path = os.environ.get("GITHUB_EVENT_PATH")
if not event_path or not os.path.exists(event_path):
    print("ERROR: GITHUB_EVENT_PATH not set or invalid", file=sys.stderr)
    sys.exit(1)

with open(event_path, 'r') as fp:
    data = json.load(fp)

# ─── Sanity check ───────────────────────────────────────────────
if pr_number is None:
    print("ERROR: Unable to determine PR number", file=sys.stderr)
    sys.exit(1)

# ─── GitHub & OpenAI Init ───────────────────────────────────────
gh = Github(os.environ["GITHUB_TOKEN"])
repo = gh.get_repo(os.environ["GITHUB_REPOSITORY"])
pr = repo.get_pull(pr_number)

openai_key = os.environ.get("OPENAI_API_KEY")
if not openai_key:
    print("ERROR: OPENAI_API_KEY not set", file=sys.stderr)
    sys.exit(1)
client = OpenAI(api_key=openai_key)

# ─── Define tool schema for review comments ────────────────────
review_tool = {
    "type": "function",
    "function": {
        "name": "submit_review_comments",
        "description": "Submit review comments for code issues found in the diff",
        "parameters": {
            "type": "object",
            "properties": {
                "comments": {
                    "type": "array",
                    "description": "List of review comments for critical issues",
                    "items": {
                        "type": "object",
                        "properties": {
                            "line": {
                                "type": "integer",
                                "description": "Line number where the issue occurs"
                            },
                            "comment": {
                                "type": "string",
                                "description": "The review comment describing the issue"
                            }
                        },
                        "required": ["line", "comment"]
                    }
                }
            },
            "required": ["comments"]
        }
    }
}

# ─── Review Patch Files ─────────────────────────────────────────
for f in pr.get_files():
    if (
        f.filename.startswith('.github/')
        or f.filename.endswith(('.md', '_test.go', '_test.py'))
    ):
        print(f"⏭️  Skipping file: {f.filename}")
        continue

    if not f.patch:
        continue

    print(f"🔍 Reviewing {f.filename}")

    try:
        response = client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "system",
                    "content": (
                        "You are a strict and concise code reviewer. "
                        "Only comment on critical issues such as logic bugs, incorrect assumptions, syntax errors, or spelling mistakes. "
                        "Do not comment on minor style or formatting issues. "
                        "Use the submit_review_comments function to provide your review. "
                        "If there are no important issues, call the function with an empty comments array."
                    )
                },
                {
                    "role": "user",
                    "content": f"Review this code diff:\n\n```diff\n{f.patch}\n```"
                }
            ],
            tools=[review_tool],
            tool_choice="required"
        )

        # Process tool calls
        tool_calls = response.choices[0].message.tool_calls
        if not tool_calls:
            print(f"✅ No tool calls made for {f.filename}")
            continue

        for tool_call in tool_calls:
            if tool_call.function.name == "submit_review_comments":
                arguments = json.loads(tool_call.function.arguments)
                comments = arguments.get("comments", [])
                
                if not comments:
                    print(f"✅ No critical issues found in {f.filename}")
                    continue

                for comment in comments:
                    pr.create_review_comment(
                        body=comment["comment"],
                        commit_id=pr.head.sha,
                        path=f.filename,
                        line=comment["line"],
                        side="RIGHT"
                    )
                print(f"💬 Posted {len(comments)} comment(s) for {f.filename}")

    except Exception as e:
        print("⚠️ Failed to process tool calls or post comments:", e)
        if hasattr(response, 'choices') and response.choices:
            print("🔎 Raw response:", response.choices[0].message)

print(f"🏁 Finished reviewing PR #{pr_number}")
