local M = {}

M.config = {
  bin = "demo-it",
  run_id = nil,
  socket = nil,
}

local function base_args()
  local args = {}
  if M.config.run_id and M.config.run_id ~= "" then
    table.insert(args, "--run-id")
    table.insert(args, M.config.run_id)
  end
  if M.config.socket and M.config.socket ~= "" then
    table.insert(args, "--socket")
    table.insert(args, M.config.socket)
  end
  return args
end

local function system_call(args)
  local result = vim.system(args, { text = true }):wait()
  if result.code == 0 then
    local output = (result.stdout or ""):gsub("%s+$", "")
    if output ~= "" then
      vim.notify(output, vim.log.levels.INFO, { title = "demo-it" })
    end
    return true
  end

  local err = result.stderr or "demo-it command failed"
  vim.notify(err:gsub("%s+$", ""), vim.log.levels.ERROR, { title = "demo-it" })
  return false
end

function M.exec(cli_args)
  local args = { M.config.bin }
  vim.list_extend(args, base_args())
  vim.list_extend(args, cli_args)
  return system_call(args)
end

function M.reload()
  package.loaded["demo-it"] = nil
  for name, _ in pairs(package.loaded) do
    if name:match("^demo%-it%.") then
      package.loaded[name] = nil
    end
  end
  return require("demo-it").setup(M.config)
end

local function set_command(name, opts)
  pcall(vim.api.nvim_del_user_command, name)
  vim.api.nvim_create_user_command(name, opts.fn, {
    nargs = opts.nargs or 0,
    complete = opts.complete,
  })
end

function M.setup(opts)
  M.config = vim.tbl_deep_extend("force", M.config, opts or {})

  set_command("DemoIt", {
    nargs = "+",
    fn = function(ctx)
      M.exec(ctx.fargs)
    end,
  })

  set_command("DemoItStart", { fn = function() M.exec({ "start" }) end })
  set_command("DemoItStatus", { fn = function() M.exec({ "status" }) end })
  set_command("DemoItReloadState", { fn = function() M.exec({ "reload" }) end })
  set_command("DemoItNext", { fn = function() M.exec({ "next" }) end })
  set_command("DemoItPrev", { fn = function() M.exec({ "prev" }) end })
  set_command("DemoItRerun", { fn = function() M.exec({ "rerun" }) end })
  set_command("DemoItJump", {
    nargs = 1,
    fn = function(ctx)
      M.exec({ "jump", "--slide", ctx.args })
    end,
  })
  set_command("DemoItFocus", {
    nargs = 1,
    complete = function()
      return { "present", "return", "none" }
    end,
    fn = function(ctx)
      M.exec({ "focus", "--policy", ctx.args })
    end,
  })
  set_command("DemoItReload", { fn = M.reload })

  return M
end

return M
