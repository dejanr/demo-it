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

local function echo_info(message)
	vim.api.nvim_echo({ { message, "ModeMsg" } }, false, {})
end

local function decode_json(text)
	local ok, decoded = pcall(vim.json.decode, text)
	if ok and type(decoded) == "table" then
		return decoded
	end
	return nil
end

local function format_transition_output(command, output)
	if output == "" then
		echo_info(("demo-it %s: done"):format(command))
		return
	end

	local state = decode_json(output)
	if not state then
		echo_info(output)
		return
	end

	if state.workspace_step_available then
		local step = tonumber(state.workspace_step)
		local total = tonumber(state.workspace_total_steps)
		if step then
			step = step + 1
		end

		local details = {}
		if step and total and total > 0 then
			table.insert(details, ("slide %d/%d"):format(step, total))
			table.insert(details, ("step %d"):format(step))
		elseif step then
			table.insert(details, ("slide %d"):format(step))
			table.insert(details, ("step %d"):format(step))
		end
		if state.workspace_step_title and state.workspace_step_title ~= "" then
			table.insert(details, state.workspace_step_title)
		end
		if #details > 0 then
			echo_info(("demo-it %s: %s"):format(command, table.concat(details, ", ")))
			return
		end
	end

	local details = {}
	local slide = tonumber(state.current_slide)
	if slide then
		table.insert(details, ("slide %d"):format(slide + 1))
	end

	local interaction = tonumber(state.current_interaction)
	if interaction and interaction >= 0 then
		local stepDetail = ("step %d"):format(interaction + 1)
		if state.interaction_id and state.interaction_id ~= "" then
			stepDetail = stepDetail .. (" (%s)"):format(state.interaction_id)
		end
		if state.skipped then
			stepDetail = stepDetail .. " skipped"
		end
		table.insert(details, stepDetail)
	else
		table.insert(details, "step -")
	end

	echo_info(("demo-it %s: %s"):format(command, table.concat(details, ", ")))
end

local function system_call(args, opts)
	opts = opts or {}
	local result = vim.system(args, { text = true }):wait()
	if result.code == 0 then
		local output = (result.stdout or ""):gsub("%s+$", "")
		if opts.on_success then
			opts.on_success(output)
			return true
		end
		if output ~= "" then
			vim.notify(output, vim.log.levels.INFO, { title = "demo-it" })
		end
		return true
	end

	local err = result.stderr or "demo-it command failed"
	vim.notify(err:gsub("%s+$", ""), vim.log.levels.ERROR, { title = "demo-it" })
	return false
end

function M.exec(cli_args, opts)
	local args = { M.config.bin }
	vim.list_extend(args, base_args())
	vim.list_extend(args, cli_args)
	return system_call(args, opts)
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

	set_command("DemoItStart", {
		fn = function()
			M.exec({ "start" })
		end,
	})
	set_command("DemoItStatus", {
		fn = function()
			M.exec({ "status" })
		end,
	})
	set_command("DemoItReloadState", {
		fn = function()
			M.exec({ "reload" })
		end,
	})
	set_command("DemoItNext", {
		fn = function()
			M.exec({ "next" }, {
				on_success = function(output)
					format_transition_output("next", output)
				end,
			})
		end,
	})
	set_command("DemoItPrev", {
		fn = function()
			M.exec({ "prev" }, {
				on_success = function(output)
					format_transition_output("prev", output)
				end,
			})
		end,
	})
	set_command("DemoItRerun", {
		fn = function()
			M.exec({ "rerun" }, {
				on_success = function(output)
					format_transition_output("rerun", output)
				end,
			})
		end,
	})
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
