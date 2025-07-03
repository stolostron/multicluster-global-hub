#!/usr/bin/env python3
"""
AI-powered code review script for GitHub Pull Requests
Automatically reviews code changes and provides intelligent feedback
"""

import os
import sys
import json
import argparse
from typing import List, Dict, Set, Optional, Tuple
from github import Github
from openai import OpenAI

# ─── Configuration Constants ─────────────────────────────────────
MAX_COMMENTS = 10
SKIP_PATTERNS = ['.github/', '.md', '_test.go', '_test.py']
OPENAI_MODEL = "gpt-4o-mini"
REVIEW_EVENT = "COMMENT"

# ─── Tool Definitions ────────────────────────────────────────────
REVIEW_TOOL = {
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
                            "line": {"type": "integer", "description": "Line number where the issue occurs"},
                            "comment": {"type": "string", "description": "The review comment describing the issue"}
                        },
                        "required": ["line", "comment"]
                    }
                }
            },
            "required": ["comments"]
        }
    }
}

PRIORITIZE_TOOL = {
    "type": "function",
    "function": {
        "name": "select_most_important_issues",
        "description": "Select the most important issues from a list of code review findings",
        "parameters": {
            "type": "object",
            "properties": {
                "selected_indices": {
                    "type": "array",
                    "description": f"Array of indices of the {MAX_COMMENTS} most important issues to prioritize",
                    "items": {"type": "integer", "description": "Index of an important issue"},
                    "maxItems": MAX_COMMENTS
                }
            },
            "required": ["selected_indices"]
        }
    }
}

class CodeReviewer:
    def __init__(self, github_token: str, openai_key: str, repo_name: str):
        self.gh = Github(github_token)
        self.repo = self.gh.get_repo(repo_name)
        self.client = OpenAI(api_key=openai_key)
        self.invalid_comments = []

    def parse_diff_lines(self, patch: str) -> Set[int]:
        """Parse diff patch to extract line numbers that can receive comments"""
        if not patch:
            return set()
        
        valid_lines = set()
        current_line = 0
        
        for line in patch.split('\n'):
            if line.startswith('@@'):
                # Parse hunk header: @@ -old_start,old_count +new_start,new_count @@
                parts = line.split()
                if len(parts) >= 3:
                    new_info = parts[2]  # +new_start,new_count
                    if new_info.startswith('+'):
                        current_line = int(new_info[1:].split(',')[0]) - 1
            elif line.startswith('+'):
                # Added line - valid for comments
                current_line += 1
                valid_lines.add(current_line)
            elif line.startswith('-'):
                # Removed line - skip (don't increment current_line)
                pass
            elif line.startswith(' '):
                # Context line - valid for comments
                current_line += 1
                valid_lines.add(current_line)
        
        return valid_lines

    def should_skip_file(self, filename: str) -> bool:
        """Check if file should be skipped based on patterns"""
        return any(filename.startswith(pattern) or filename.endswith(pattern) 
                  for pattern in SKIP_PATTERNS)

    def review_file(self, file_obj) -> List[Dict]:
        """Review a single file and return valid comments"""
        if self.should_skip_file(file_obj.filename):
            print(f"⏭️  Skipping file: {file_obj.filename}")
            return []

        if not file_obj.patch:
            return []

        print(f"🔍 Reviewing {file_obj.filename}")
        
        try:
            # Get AI review
            response = self.client.chat.completions.create(
                model=OPENAI_MODEL,
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You are a strict and concise code reviewer. "
                            "Only comment on critical issues such as logic bugs, incorrect assumptions, "
                            "syntax errors, or spelling mistakes. Do not comment on minor style or formatting issues. "
                            "Use the submit_review_comments function to provide your review. "
                            "If there are no important issues, call the function with an empty comments array."
                        )
                    },
                    {
                        "role": "user",
                        "content": f"Review this code diff:\n\n```diff\n{file_obj.patch}\n```"
                    }
                ],
                tools=[REVIEW_TOOL],
                tool_choice="required"
            )

            # Process tool calls
            tool_calls = response.choices[0].message.tool_calls
            if not tool_calls:
                print(f"✅ No tool calls made for {file_obj.filename}")
                return []

            # Extract comments from tool calls
            all_comments = []
            for tool_call in tool_calls:
                if tool_call.function.name == "submit_review_comments":
                    arguments = json.loads(tool_call.function.arguments)
                    comments = arguments.get("comments", [])
                    all_comments.extend(comments)

            if not all_comments:
                print(f"✅ No critical issues found in {file_obj.filename}")
                return []

            # Validate comments against diff
            return self.validate_comments(file_obj, all_comments)

        except Exception as e:
            print(f"⚠️ Failed to review {file_obj.filename}: {e}")
            return []

    def validate_comments(self, file_obj, comments: List[Dict]) -> List[Dict]:
        """Validate comments against diff and return valid ones"""
        valid_lines = self.parse_diff_lines(file_obj.patch)
        valid_comments = []
        invalid_count = 0

        for comment in comments:
            line_num = comment["line"]
            if line_num in valid_lines:
                valid_comments.append({
                    "path": file_obj.filename,
                    "body": comment["comment"],
                    "line": line_num,
                    "side": "RIGHT"
                })
            else:
                self.invalid_comments.append({
                    "path": file_obj.filename,
                    "line": line_num,
                    "comment": comment["comment"]
                })
                invalid_count += 1

        print(f"📝 Found {len(comments)} issues in {file_obj.filename} "
              f"({len(valid_comments)} valid, {invalid_count} invalid)")
        return valid_comments

    def prioritize_comments(self, comments: List[Dict]) -> List[Dict]:
        """Select the most important comments if there are too many"""
        if len(comments) <= MAX_COMMENTS:
            return comments

        print(f"🔍 More than {MAX_COMMENTS} issues found, selecting the most important ones...")
        
        try:
            # Prepare issues for selection
            issues_for_selection = [
                {
                    "index": i,
                    "file": comment["path"],
                    "line": comment["line"],
                    "issue": comment["body"]
                }
                for i, comment in enumerate(comments)
            ]

            response = self.client.chat.completions.create(
                model=OPENAI_MODEL,
                messages=[
                    {
                        "role": "system",
                        "content": (
                            "You are a code review prioritizer. "
                            f"Select the {MAX_COMMENTS} most important issues from the provided list. "
                            "Prioritize: 1) Logic bugs, 2) Security issues, 3) Performance problems, "
                            "4) Correctness issues, 5) Other critical problems. "
                            "Use the select_most_important_issues function to provide your selection."
                        )
                    },
                    {
                        "role": "user",
                        "content": (
                            f"Select the {MAX_COMMENTS} most important issues from these "
                            f"{len(issues_for_selection)} code review findings:\n\n" +
                            "\n".join([f"Index {issue['index']}: {issue['file']}:L{issue['line']} - {issue['issue']}" 
                                     for issue in issues_for_selection])
                        )
                    }
                ],
                tools=[PRIORITIZE_TOOL],
                tool_choice="required"
            )

            # Process tool calls for prioritization
            tool_calls = response.choices[0].message.tool_calls
            if tool_calls and tool_calls[0].function.name == "select_most_important_issues":
                arguments = json.loads(tool_calls[0].function.arguments)
                selected_indices = arguments.get("selected_indices", [])
                
                # Filter to selected comments
                selected_comments = [comments[i] for i in selected_indices if i < len(comments)]
                result = selected_comments[:MAX_COMMENTS]  # Ensure we don't exceed MAX_COMMENTS
                
                print(f"✅ Selected {len(result)} most important issues")
                return result

        except Exception as e:
            print(f"⚠️ Failed to prioritize issues: {e}")
            print(f"🔄 Using first {MAX_COMMENTS} issues as fallback")
        
        return comments[:MAX_COMMENTS]

    def generate_review_body(self, comments: List[Dict], reviewed_files: List[str]) -> str:
        """Generate meaningful review body using AI"""
        try:
            response = self.client.chat.completions.create(
                model=OPENAI_MODEL,
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
                            f"Total issues found: {len(comments)}\n\n"
                            f"Issues:\n" + 
                            "\n".join([f"- {comment['path']}:L{comment['line']}: {comment['body']}" 
                                     for comment in comments])
                        )
                    }
                ]
            )
            
            return response.choices[0].message.content
            
        except Exception as e:
            print(f"⚠️ Failed to generate review body: {e}")
            return f"Automated code review found {len(comments)} issues across {len(reviewed_files)} files."

    def create_review(self, pr, review_body: str, comments: List[Dict]) -> bool:
        """Create GitHub review with comments"""
        try:
            if comments:
                pr.create_review(
                    body=review_body,
                    event=REVIEW_EVENT,
                    comments=comments
                )
                print(f"✅ Successfully created review with {len(comments)} valid comments")
                return True
            else:
                # No valid comments for review, just post the summary
                pr.create_issue_comment(body=f"## 🤖 Automated Code Review Summary\n\n{review_body}")
                print("✅ Posted review summary (no valid diff lines for inline comments)")
                return True
                
        except Exception as e:
            print(f"❌ Failed to create review: {e}")
            return False

    def create_fallback_comments(self, pr, review_body: str, comments: List[Dict]):
        """Create issue comments as fallback"""
        print("🔄 Falling back to issue comments...")
        try:
            # Post review summary
            pr.create_issue_comment(body=f"## 🤖 Automated Code Review Summary\n\n{review_body}")
            
            # Post individual comments
            for comment in comments:
                pr.create_issue_comment(
                    body=f"**📍 `{comment['path']}:L{comment['line']}`**\n\n{comment['body']}"
                )
            
            print(f"✅ Posted summary + {len(comments)} individual comments")
            
        except Exception as e:
            print(f"❌ Failed to create fallback comments: {e}")

    def review_pull_request(self, pr_number: int):
        """Main method to review a pull request"""
        pr = self.repo.get_pull(pr_number)
        print(f"🔍 Starting review of PR #{pr_number}")
        
        # Review all files
        all_comments = []
        reviewed_files = []
        
        for file_obj in pr.get_files():
            file_comments = self.review_file(file_obj)
            if file_comments:
                all_comments.extend(file_comments)
                reviewed_files.append(file_obj.filename)
        
        if not all_comments:
            print("✅ No critical issues found in any reviewed files")
            return
        
        print(f"📋 Total comments found: {len(all_comments)} across {len(reviewed_files)} files")
        
        # Prioritize comments if too many
        all_comments = self.prioritize_comments(all_comments)
        
        # Generate review body
        review_body = self.generate_review_body(all_comments, reviewed_files)
        print(f"📝 Generated review body: {review_body}")
        
        # Try to create review
        if not self.create_review(pr, review_body, all_comments):
            self.create_fallback_comments(pr, review_body, all_comments)
        
        print(f"🏁 Finished reviewing PR #{pr_number}")

def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(description="AI-powered code review for GitHub PRs")
    parser.add_argument("--pr", type=int, required=True, help="PR number to review")
    args = parser.parse_args()

    # Load environment variables
    event_path = os.environ.get("GITHUB_EVENT_PATH")
    if not event_path or not os.path.exists(event_path):
        print("ERROR: GITHUB_EVENT_PATH not set or invalid", file=sys.stderr)
        sys.exit(1)

    github_token = os.environ.get("GITHUB_TOKEN")
    if not github_token:
        print("ERROR: GITHUB_TOKEN not set", file=sys.stderr)
        sys.exit(1)

    openai_key = os.environ.get("OPENAI_API_KEY")
    if not openai_key:
        print("ERROR: OPENAI_API_KEY not set", file=sys.stderr)
        sys.exit(1)

    repo_name = os.environ.get("GITHUB_REPOSITORY")
    if not repo_name:
        print("ERROR: GITHUB_REPOSITORY not set", file=sys.stderr)
        sys.exit(1)

    # Create reviewer and run
    reviewer = CodeReviewer(github_token, openai_key, repo_name)
    reviewer.review_pull_request(args.pr)

if __name__ == "__main__":
    main()
