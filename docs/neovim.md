# Neovim

Launch the runner in another tmux, Zellij, or terminal pane:

```sh
runner --manual --name aoc -- python solve.py --input sample.txt
```

Save modified buffers and poke the runner with `<leader>r` (Neovim 0.10+):

```lua
vim.keymap.set("n", "<leader>r", function()
  vim.cmd("wall")
  vim.system({ "runner", "poke", "aoc" }, {}, function(result)
    if result.code ~= 0 then
      vim.schedule(function()
        vim.notify(result.stderr, vim.log.levels.WARN)
      end)
    end
  end)
end, { desc = "Run aoc" })
```

Optional autosave on leaving insert mode or focusing another window:

```lua
vim.api.nvim_create_autocmd({ "InsertLeave", "FocusLost" }, {
  group = vim.api.nvim_create_augroup("runner_autosave", { clear = true }),
  callback = function()
    if vim.bo.buftype == "" and vim.bo.modifiable and not vim.bo.readonly
      and vim.bo.modified and vim.api.nvim_buf_get_name(0) ~= "" then
      vim.cmd("silent update")
    end
  end,
})
```

Manual mode keeps autosaves separate from executions. Choose a different runner
name for each simultaneous task. The mapping works on Linux and Windows without
knowing a pane ID or PID. The command runs saved files, not unsaved buffers.
