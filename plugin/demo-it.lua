if vim.g.loaded_demo_it_plugin == 1 then
	return
end
vim.g.loaded_demo_it_plugin = 1

require("demo-it").setup()
