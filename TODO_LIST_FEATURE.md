# !todo-list Feature Documentation

## Overview
The `!todo-list` command provides an interactive pagination system for viewing user tasks in Discord. It fetches tasks from a backend API and displays them with navigation buttons for seamless page browsing.

## Command Flow

### 1. Initial Command Execution
When a user types `!todo-list`:
1. Bot sets current page to 1 (default)
2. Checks if user has existing pagination state (uses existing page if found)
3. Calls API: `GET http://localhost:8080/api/task/user?discord_id={user_id}&page=1&limit=5`
4. Processes API response to display tasks

### 2. Display Logic
- Shows tasks in numbered list format with status emojis:
  - 📝 Default/New tasks
  - ✅ Done tasks
  - 🔄 In-progress tasks
  - 📥 Backlog tasks
- Displays pagination info: "Page X of Y | Total tasks: Z"

### 3. Button Rendering
Buttons are dynamically shown based on current page:
- Previous button: Shown when page > 1
- Next button: Shown when page < total_pages
- Buttons use custom IDs: `todo_prev_N` or `todo_next_N` where N is target page

## Interaction Flow

### Button Click Processing
When user clicks a navigation button:
1. Discord sends interaction event with button's custom ID
2. Bot parses ID to extract target page (e.g., "todo_next_2" → page 2)
3. Updates user's pagination state in memory
4. Fetches tasks for new page from API
5. Updates original message with new tasks and buttons

### State Management
- Each user has independent pagination state stored in `userPagination` map
- State persists during bot session (resets on restart)
- Prevents users from interfering with each other's navigation

## API Integration

### Request Format
```
GET http://localhost:8080/api/task/user?discord_id={id}&page={num}&limit=5
```

### Response Structure
```json
{
  "tasks": [
    {
      "id": "uuid",
      "title": "Task Name",
      "status": "backlog|in-progress|done"
    }
  ],
  "page": 1,
  "limit": 5,
  "total": 12,
  "total_pages": 3
}
```

## Technical Implementation

### Key Components
1. **PaginationState struct** - Tracks current page per user
2. **userPagination map** - Stores states for all users
3. **Message Components** - Discord buttons for navigation
4. **Interaction Handler** - Processes button clicks

### Error Handling
- API errors displayed to user
- Invalid responses gracefully handled
- Missing data shown as empty states

### User Experience
- Immediate feedback on button clicks
- No new messages cluttering chat
- Clear visual indicators for task statuses
- Intuitive navigation controls