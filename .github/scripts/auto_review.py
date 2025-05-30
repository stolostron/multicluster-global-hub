import os
import sys
import json
from github import Github
from openai import OpenAI

import argparse

parser = argparse.ArgumentParser()
parser.add_argument("--pr", type=int, help="PR number to review")
args = parser.parse_args()

# if args.pr:
#     pr_number = args.pr
# else:

pr_number = args.pr

# ─── Load webhook/event payload ───────────────────────────────────────────────
event_path = os.environ.get("GITHUB_EVENT_PATH")
if not event_path or not os.path.exists(event_path):
    print("ERROR: GITHUB_EVENT_PATH not set or invalid", file=sys.stderr)
    sys.exit(1)

with open(event_path, 'r') as fp:
    data = json.load(fp)
    print(data)


# sanity check
if pr_number is None:
    print("ERROR: Unable to determine PR number", file=sys.stderr)
    sys.exit(1)

# ─── Initialize GitHub & OpenAI clients ───────────────────────────────────────
gh = Github(os.environ["GITHUB_TOKEN"])
repo = gh.get_repo(os.environ["GITHUB_REPOSITORY"])
pr = repo.get_pull(pr_number)

openai_key = os.environ.get("OPENAI_API_KEY")
if not openai_key:
    print("ERROR: OPENAI_API_KEY not set", file=sys.stderr)
    sys.exit(1)
client = OpenAI(api_key=openai_key)

# ─── Gather diffs and post comments ────────────────────────────────────────────
for f in pr.get_files():
    # Skip workflow files
    if f.filename.startswith('.github/'):
        print(f"⏭️  Skipping workflow file: {f.filename}")
        continue
    
    # Only review Python files with changes
    if not f.filename.endswith(".py") or not f.patch:
        continue

    diff = f.patch
    response = client.chat.completions.create(
        model="gpt-4o-mini",
        messages=[
            {"role": "system", "content": "You are a code review assistant."},
            {"role": "user", "content": f"Please review this patch and suggest improvements:\n\n```diff\n{diff}\n```"},
        ]
    )

    review_text = response.choices[0].message.content.strip()
    pr.create_review(body=review_text, event="COMMENT")

print(f"✅ Posted review comments on PR #{pr_number}")