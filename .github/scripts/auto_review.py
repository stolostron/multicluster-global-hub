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

# ─── Define tool schema for prioritizing issues ────────────────
prioritize_tool = {
    "type": "function",
    "function": {
        "name": "select_most_important_issues",
        "description": "Select the 10 most important issues from a list of code review findings",
        "parameters": {
            "type": "object",
            "properties": {
                "selected_indices": {
                    "type": "array",
                    "description": "Array of indices (0-based) of the 10 most important issues to prioritize",
                    "items": {
                        "type": "integer",
                        "description": "Index of an important issue"
                    },
                    "maxItems": 10
                }
            },
            "required": ["selected_indices"]
        }
    }
}

# ─── Collect all files and review them ─────────────────────────────
all_review_comments = []
reviewed_files = []

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
    reviewed_files.append(f.filename)

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

                # Add comments to the master list
                for comment in comments:
                    all_review_comments.append({
                        "path": f.filename,
                        "body": comment["comment"],
                        "line": comment["line"],
                        "side": "RIGHT"
                    })
                
                print(f"📝 Found {len(comments)} issues in {f.filename}")

    except Exception as e:
        print("⚠️ Failed to process tool calls:", e)
        if hasattr(response, 'choices') and response.choices:
            print("🔎 Raw response:", response.choices[0].message)

# ─── Generate review body and create combined review ─────────────
if all_review_comments:
    print(f"📋 Total comments found: {len(all_review_comments)} across {len(reviewed_files)} files")
    
    # If more than 10 comments, select the most important ones
    if len(all_review_comments) > 10:
        print("🔍 More than 10 issues found, selecting the 10 most important ones...")
        try:
            # Prepare the issues for selection
            issues_for_selection = []
            for i, comment in enumerate(all_review_comments):
                issues_for_selection.append({
                    "index": i,
                    "file": comment["path"],
                    "line": comment["line"],
                    "issue": comment["body"]
                })
            
            selection_response = client.chat.completions.create(
                model="gpt-4o-mini",
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You are a code review prioritizer. "
                            "Select the 10 most important issues from the provided list. "
                            "Prioritize: 1) Logic bugs, 2) Security issues, 3) Performance problems, 4) Correctness issues, 5) Other critical problems. "
                            "Use the select_most_important_issues function to provide your selection."
                        )
                    },
                    {
                        "role": "user",
                        "content": (
                            f"Select the 10 most important issues from these {len(issues_for_selection)} code review findings:\n\n" +
                            "\n".join([f"Index {issue['index']}: {issue['file']}:L{issue['line']} - {issue['issue']}" 
                                     for issue in issues_for_selection])
                        )
                    }
                ],
                tools=[prioritize_tool],
                tool_choice="required"
            )
            
            # Process tool calls for prioritization
            tool_calls = selection_response.choices[0].message.tool_calls
            if tool_calls and tool_calls[0].function.name == "select_most_important_issues":
                arguments = json.loads(tool_calls[0].function.arguments)
                selected_indices = arguments.get("selected_indices", [])
            else:
                selected_indices = []
            
            # Filter to selected comments
            selected_comments = [all_review_comments[i] for i in selected_indices if i < len(all_review_comments)]
            all_review_comments = selected_comments[:10]  # Ensure we don't exceed 10
            
            print(f"✅ Selected {len(all_review_comments)} most important issues")
            
        except Exception as e:
            print(f"⚠️ Failed to prioritize issues: {e}")
            print("🔄 Using first 10 issues as fallback")
            all_review_comments = all_review_comments[:10]
    
    # Generate meaningful review body
    try:
        body_response = client.chat.completions.create(
            model="gpt-4o-mini",
            messages=[
                {
                    "role": "system",
                    "content": (
                        "You are a code review summarizer. "
                        "Generate a concise summary of the code review findings. "
                        "Focus on the main themes and critical issues found. "
                        "Keep it professional and constructive."
                    )
                },
                {
                    "role": "user",
                    "content": (
                        f"Generate a summary for a code review with the following findings:\n\n"
                        f"Files reviewed: {', '.join(reviewed_files)}\n"
                        f"Total issues found: {len(all_review_comments)}\n\n"
                        f"Issues:\n" + 
                        "\n".join([f"- {comment['path']}:L{comment['line']}: {comment['body']}" 
                                 for comment in all_review_comments])
                    )
                }
            ]
        )
        
        review_body = body_response.choices[0].message.content
        print(f"📝 Generated review body: {review_body}")
        
    except Exception as e:
        print(f"⚠️ Failed to generate review body: {e}")
        review_body = f"Automated code review found {len(all_review_comments)} issues across {len(reviewed_files)} files."
    
    # Create the combined review
    try:
        pr.create_review(
            body=review_body,
            event="COMMENT",
            comments=all_review_comments
        )
        print(f"✅ Successfully created combined review with {len(all_review_comments)} comments")
    except Exception as e:
        print(f"❌ Failed to create combined review: {e}")
        print("🔄 Falling back to issue comments...")
        for comment in all_review_comments:
            pr.create_issue_comment(
                body=f"**🤖 Code Review Comment for `{comment['path']}:L{comment['line']}`**\n\n{comment['body']}"
            )
        print(f"✅ Posted {len(all_review_comments)} individual comments")
else:
    print("✅ No critical issues found in any reviewed files")

print(f"🏁 Finished reviewing PR #{pr_number}")
