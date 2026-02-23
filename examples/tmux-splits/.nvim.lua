local workspace = vim.fn.fnamemodify(debug.getinfo(1, "S").source:sub(2), ":p:h")
local root = vim.fn.fnamemodify(workspace, ":h:h")

if not vim.tbl_contains(vim.opt.runtimepath:get(), root) then
	vim.opt.runtimepath:append(root)
end

require("demo-it").setup()
