-- A small Lua module with local and global functions for navigation and search.
local function format_user(user)
  return user.name .. " (" .. user.id .. ")"
end

function report(user)
  print(format_user(user))
end

return { report = report }
