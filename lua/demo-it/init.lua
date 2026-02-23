local M = {}

M.config = {
	bin = "demo-it",
	run_id = nil,
	socket = nil,
	presentation = {
		auto_enable_on_start = true,
		disable_markdown_diagnostics = true,
		global_options = {
			laststatus = 0,
			showtabline = 0,
			showmode = false,
			ruler = false,
			showcmd = false,
		},
		window_options = {
			number = false,
			relativenumber = false,
			signcolumn = "no",
			foldcolumn = "2",
			cursorline = false,
			cursorcolumn = false,
			list = false,
			wrap = true,
			linebreak = true,
			breakindent = true,
			spell = false,
			colorcolumn = "",
			winbar = " ",
			scrolloff = 4,
			sidescrolloff = 8,
		},
		markdown = {
			rendered_conceallevel = 2,
			rendered_concealcursor = "nc",
			raw_conceallevel = 0,
			raw_concealcursor = "",
		},
		font = {
			neovide_scale = 1.15,
			kitty_delta = 2,
		},
		neovide_scale = 1.15,
		kitty_font_delta = 2,
	},
}

M.presentation_state = {
	enabled = false,
	group_id = nil,
	saved_globals = {},
	saved_windows = {},
	disabled_diagnostic_buffers = {},
	saved_neovide_scale = nil,
	kitty_font_applied = false,
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
	elseif state.interaction_id and state.interaction_id ~= "" then
		local interactionDetail = ("interaction %s"):format(state.interaction_id)
		if state.skipped then
			interactionDetail = interactionDetail .. " skipped"
		end
		table.insert(details, interactionDetail)
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

local function current_working_directory()
	if vim.uv and vim.uv.cwd then
		return vim.uv.cwd()
	end
	if vim.loop and vim.loop.cwd then
		return vim.loop.cwd()
	end
	return nil
end

local function get_global_option(name)
	local ok, value = pcall(function()
		return vim.o[name]
	end)
	if ok then
		return value
	end
	return nil
end

local function set_global_option(name, value)
	pcall(function()
		vim.o[name] = value
	end)
end

local function window_option_names()
	local names = {}
	for name, _ in pairs(M.config.presentation.window_options) do
		names[#names + 1] = name
	end
	names[#names + 1] = "conceallevel"
	names[#names + 1] = "concealcursor"
	return names
end

local function should_manage_window(win)
	if not vim.api.nvim_win_is_valid(win) then
		return false
	end
	local ok, cfg = pcall(vim.api.nvim_win_get_config, win)
	if not ok or not cfg then
		return false
	end
	return cfg.relative == ""
end

local function get_window_option(win, name)
	local ok, value = pcall(function()
		return vim.wo[win][name]
	end)
	if ok then
		return value
	end
	return nil
end

local function set_window_option(win, name, value)
	pcall(function()
		vim.wo[win][name] = value
	end)
end

local function snapshot_window(win)
	if M.presentation_state.saved_windows[win] then
		return
	end
	local snapshot = {}
	for _, name in ipairs(window_option_names()) do
		local value = get_window_option(win, name)
		if value ~= nil then
			snapshot[name] = value
		end
	end
	M.presentation_state.saved_windows[win] = snapshot
end

local function is_markdown_window(win)
	if not should_manage_window(win) then
		return false
	end
	local buf = vim.api.nvim_win_get_buf(win)
	if not vim.api.nvim_buf_is_valid(buf) then
		return false
	end
	return vim.bo[buf].filetype == "markdown"
end

local function diagnostics_enabled_for_buffer(buf)
	if not vim.diagnostic or type(vim.diagnostic.is_enabled) ~= "function" then
		return true
	end

	local ok, enabled = pcall(vim.diagnostic.is_enabled, { bufnr = buf })
	if ok then
		return enabled
	end

	ok, enabled = pcall(vim.diagnostic.is_enabled, buf)
	if ok then
		return enabled
	end

	return true
end

local function disable_markdown_diagnostics_for_window(win)
	if not M.config.presentation.disable_markdown_diagnostics then
		return
	end
	if not is_markdown_window(win) then
		return
	end
	if not vim.diagnostic or type(vim.diagnostic.disable) ~= "function" then
		return
	end

	local buf = vim.api.nvim_win_get_buf(win)
	if not vim.api.nvim_buf_is_valid(buf) then
		return
	end
	if M.presentation_state.disabled_diagnostic_buffers[buf] then
		return
	end
	if diagnostics_enabled_for_buffer(buf) == false then
		return
	end

	local ok = pcall(vim.diagnostic.disable, buf)
	if ok then
		M.presentation_state.disabled_diagnostic_buffers[buf] = true
	end
end

local function restore_diagnostics()
	if not vim.diagnostic or type(vim.diagnostic.enable) ~= "function" then
		M.presentation_state.disabled_diagnostic_buffers = {}
		return
	end

	for buf, _ in pairs(M.presentation_state.disabled_diagnostic_buffers) do
		if vim.api.nvim_buf_is_valid(buf) then
			pcall(vim.diagnostic.enable, buf)
		end
	end
	M.presentation_state.disabled_diagnostic_buffers = {}
end

local function apply_markdown_render_mode(win, rendered)
	if not is_markdown_window(win) then
		return
	end
	if rendered then
		set_window_option(win, "conceallevel", M.config.presentation.markdown.rendered_conceallevel)
		set_window_option(win, "concealcursor", M.config.presentation.markdown.rendered_concealcursor)
		return
	end
	set_window_option(win, "conceallevel", M.config.presentation.markdown.raw_conceallevel)
	set_window_option(win, "concealcursor", M.config.presentation.markdown.raw_concealcursor)
end

local function apply_window_presentation(win)
	if not should_manage_window(win) then
		return
	end
	snapshot_window(win)
	for name, value in pairs(M.config.presentation.window_options) do
		set_window_option(win, name, value)
	end
	disable_markdown_diagnostics_for_window(win)
	apply_markdown_render_mode(win, true)
end

local function apply_all_windows_presentation()
	for _, win in ipairs(vim.api.nvim_list_wins()) do
		apply_window_presentation(win)
	end
end

local function restore_windows()
	for win, options in pairs(M.presentation_state.saved_windows) do
		if vim.api.nvim_win_is_valid(win) then
			for name, value in pairs(options) do
				set_window_option(win, name, value)
			end
		end
	end
	M.presentation_state.saved_windows = {}
end

local function apply_global_presentation()
	for name, value in pairs(M.config.presentation.global_options) do
		if M.presentation_state.saved_globals[name] == nil then
			M.presentation_state.saved_globals[name] = get_global_option(name)
		end
		set_global_option(name, value)
	end
end

local function restore_global_presentation()
	for name, value in pairs(M.presentation_state.saved_globals) do
		if value ~= nil then
			set_global_option(name, value)
		end
	end
	M.presentation_state.saved_globals = {}
end

local function presentation_font_config()
	if type(M.config.presentation.font) == "table" then
		return M.config.presentation.font
	end
	return {}
end

local function configured_neovide_scale()
	local font = presentation_font_config()
	if font.neovide_scale ~= nil then
		return tonumber(font.neovide_scale) or 1
	end
	if M.config.presentation.neovide_scale ~= nil then
		return tonumber(M.config.presentation.neovide_scale) or 1
	end
	return 1
end

local function configured_kitty_font_delta()
	local font = presentation_font_config()
	if font.kitty_delta ~= nil then
		return math.floor(tonumber(font.kitty_delta) or 0)
	end
	if M.config.presentation.kitty_font_delta ~= nil then
		return math.floor(tonumber(M.config.presentation.kitty_font_delta) or 0)
	end
	return 0
end

local function apply_neovide_scale()
	if vim.g.neovide ~= true and vim.g.neovide ~= 1 then
		return
	end
	if type(vim.g.neovide_scale_factor) ~= "number" then
		return
	end
	local scale = configured_neovide_scale()
	if scale <= 0 then
		return
	end
	if M.presentation_state.saved_neovide_scale == nil then
		M.presentation_state.saved_neovide_scale = vim.g.neovide_scale_factor
	end
	vim.g.neovide_scale_factor = M.presentation_state.saved_neovide_scale * scale
end

local function restore_neovide_scale()
	if M.presentation_state.saved_neovide_scale == nil then
		return
	end
	vim.g.neovide_scale_factor = M.presentation_state.saved_neovide_scale
	M.presentation_state.saved_neovide_scale = nil
end

local function adjust_kitty_font(delta)
	if delta == 0 then
		return false
	end
	if vim.env.KITTY_LISTEN_ON == nil or vim.env.KITTY_LISTEN_ON == "" then
		return false
	end
	if vim.fn.executable("kitty") ~= 1 then
		return false
	end
	local sign = "+"
	if delta < 0 then
		sign = ""
	end
	local result = vim.system({ "kitty", "@", "set-font-size", sign .. tostring(delta) }, { text = true }):wait()
	return result.code == 0
end

local function apply_terminal_font()
	local delta = configured_kitty_font_delta()
	if delta <= 0 then
		return
	end
	if adjust_kitty_font(delta) then
		M.presentation_state.kitty_font_applied = true
	end
end

local function restore_terminal_font()
	if not M.presentation_state.kitty_font_applied then
		return
	end
	local delta = configured_kitty_font_delta()
	if delta > 0 then
		adjust_kitty_font(-delta)
	end
	M.presentation_state.kitty_font_applied = false
end

local function command_exists(name)
	return vim.fn.exists(":" .. name) == 2
end

local function clear_presentation_autocmds()
	if M.presentation_state.group_id == nil then
		return
	end
	pcall(vim.api.nvim_del_augroup_by_id, M.presentation_state.group_id)
	M.presentation_state.group_id = nil
end

local function setup_presentation_autocmds()
	clear_presentation_autocmds()
	local group = vim.api.nvim_create_augroup("DemoItPresentation", { clear = true })
	M.presentation_state.group_id = group

	vim.api.nvim_create_autocmd({ "WinEnter", "BufWinEnter", "TabEnter" }, {
		group = group,
		callback = function()
			if not M.presentation_state.enabled then
				return
			end
			apply_window_presentation(vim.api.nvim_get_current_win())
		end,
	})

	vim.api.nvim_create_autocmd("InsertEnter", {
		group = group,
		callback = function()
			if not M.presentation_state.enabled then
				return
			end
			apply_markdown_render_mode(vim.api.nvim_get_current_win(), false)
		end,
	})

	vim.api.nvim_create_autocmd("InsertLeave", {
		group = group,
		callback = function()
			if not M.presentation_state.enabled then
				return
			end
			apply_markdown_render_mode(vim.api.nvim_get_current_win(), true)
		end,
	})
end

local function activate_builtin_presentation_mode()
	apply_global_presentation()
	apply_all_windows_presentation()
	setup_presentation_autocmds()
end

local function deactivate_builtin_presentation_mode()
	clear_presentation_autocmds()
	restore_windows()
	restore_diagnostics()
	restore_global_presentation()
end

function M.enable_presentation_mode()
	if M.presentation_state.enabled then
		return
	end
	M.presentation_state.enabled = true

	activate_builtin_presentation_mode()
	apply_neovide_scale()
	apply_terminal_font()
end

function M.disable_presentation_mode()
	if not M.presentation_state.enabled then
		return
	end

	deactivate_builtin_presentation_mode()
	restore_neovide_scale()
	restore_terminal_font()

	M.presentation_state.enabled = false
end

function M.toggle_presentation_mode()
	if M.presentation_state.enabled then
		M.disable_presentation_mode()
		return
	end
	M.enable_presentation_mode()
end

local function start_preview()
	if command_exists("LivePreview") then
		local ok = pcall(vim.cmd, "LivePreview start")
		if ok then
			return
		end
	end
	if command_exists("MarkdownPreview") then
		pcall(vim.cmd, "MarkdownPreview")
		return
	end
	vim.notify(
		"install live-preview.nvim or markdown-preview.nvim to enable browser preview",
		vim.log.levels.WARN,
		{ title = "demo-it" }
	)
end

local function stop_preview()
	if command_exists("LivePreview") then
		if pcall(vim.cmd, "LivePreview close") then
			return
		end
		pcall(vim.cmd, "LivePreview stop")
		return
	end
	if command_exists("MarkdownPreviewStop") then
		pcall(vim.cmd, "MarkdownPreviewStop")
		return
	end
	vim.notify("no active preview command found", vim.log.levels.INFO, { title = "demo-it" })
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
		nargs = "?",
		fn = function(ctx)
			local workspace = vim.trim(ctx.args or "")
			if workspace == "" then
				workspace = current_working_directory() or ""
			end
			if workspace == "" then
				vim.notify("could not determine current working directory", vim.log.levels.ERROR, { title = "demo-it" })
				return
			end
			M.exec({ "start", workspace }, {
				on_success = function(output)
					if M.config.presentation.auto_enable_on_start then
						M.enable_presentation_mode()
					end
					if output ~= "" then
						vim.notify(output, vim.log.levels.INFO, { title = "demo-it" })
					end
				end,
			})
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
	set_command("DemoItPresentationEnable", { fn = M.enable_presentation_mode })
	set_command("DemoItPresentationDisable", { fn = M.disable_presentation_mode })
	set_command("DemoItPresentationToggle", { fn = M.toggle_presentation_mode })
	set_command("DemoItPreview", { fn = start_preview })
	set_command("DemoItPreviewStop", { fn = stop_preview })
	set_command("DemoItReload", { fn = M.reload })

	return M
end

return M
