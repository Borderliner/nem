-- nem's xmake build.
--
--   xmake                      build a static, stripped nem (release mode)
--   xmake f -m debug; xmake    build with symbols, for a debugger
--   xmake f --goos=windows --goarch=arm64; xmake
--                              cross-compile, as the release workflow does
--   xmake run nem [file...]    run what was built
--   xmake test                 gofmt, go vet, and the tests with the race detector
--   xmake test nem/unit        one of those
--   xmake bench                every benchmark; see xmake bench --help
--   xmake install              copy nem into the install prefix's bin
--
-- xmake drives the Go toolchain rather than compiling Go itself: `go build`
-- already knows the module graph, the build cache and every platform's
-- linker, and xmake's own Go rules do not handle modules.

set_project("nem")
set_xmakever("2.8.5") -- on_test and add_tests

add_rules("mode.release", "mode.debug")
set_defaultmode("release")

option("goos")
    set_default("")
    set_showmenu(true)
    set_description("Build for this GOOS (linux, darwin, windows, freebsd...); empty is the host's.")
option_end()

option("goarch")
    set_default("")
    set_showmenu(true)
    set_description("Build for this GOARCH (amd64, arm64...); empty is the host's.")
option_end()

option("nem_version")
    set_default("")
    set_showmenu(true)
    set_description("Version stamped into the binary; empty means git describe, or dev.")
option_end()

-- Every go command runs with cgo set one way or the other, never left to Go.
--
-- A build turns it off. Go enables cgo whenever a C compiler is present, and a
-- Lip Gloss dependency pulls in os/user, which then links libc for NSS lookups:
-- with cgo on, nem is dynamically linked. With it off, nem is fully static -
-- no libc, no glibc version skew, runs on musl and Alpine.
--
-- The tests turn it on. The race detector needs cgo, and the test binary is
-- never shipped, so how it links does not matter.
--
-- (Each script builds its own environment: functions defined out here are not
-- visible inside xmake's script sandbox.)

target("nem")
    set_kind("binary")
    set_default(true)

    on_load(function (target)
        local goos, goarch = get_config("goos") or "", get_config("goarch") or ""
        -- A cross build goes under the platform it is for, in Go's names,
        -- not under the host's: build/windows/arm64/release/nem.exe.
        if goos ~= "" or goarch ~= "" then
            local function goenv(name, set)
                if set ~= "" then
                    return set
                end
                return os.iorunv("go", {"env", name}):trim()
            end
            target:set("targetdir", path.join("build", goenv("GOOS", goos), goenv("GOARCH", goarch), get_config("mode")))
        end
        -- A Windows build needs the extension, or it will not run there.
        if goos == "windows" or goos == "" and is_host("windows") then
            target:set("extension", ".exe")
        end
    end)

    on_build(function (target)
        import("lib.detect.find_program")
        local ver = get_config("nem_version")
        if not ver or ver == "" then
            ver = "dev"
            if find_program("git") then
                local out = try { function ()
                    return os.iorunv("git", {"describe", "--tags", "--always", "--dirty"})
                end }
                if out and out:trim() ~= "" then
                    ver = out:trim()
                end
            end
        end

        -- Release strips: -s drops the symbol table and -w the DWARF debug
        -- info, 7.4MB down to 5.1MB. Debug keeps both, for a debugger.
        -- -trimpath keeps this machine's paths out of the binary either way.
        local ldflags = "-X main.version=" .. ver
        if is_mode("release") then
            ldflags = "-s -w " .. ldflags
        end
        local args = {"build", "-trimpath", "-ldflags", ldflags}
        if is_mode("debug") then
            table.join2(args, {"-gcflags", "all=-N -l"})
        end
        table.join2(args, {"-o", target:targetfile(), "./cmd/nem"})

        -- A cross-compilation target, when one is configured.
        local envs = {CGO_ENABLED = "0"}
        for _, name in ipairs({"goos", "goarch"}) do
            local v = get_config(name)
            if v and v ~= "" then
                envs[name:upper()] = v
            end
        end

        os.mkdir(target:targetdir())
        cprint("${color.build.target}go build %s (%s)", target:targetfile(), ver)
        os.execv("go", args, {envs = envs})
    end)

    on_clean(function (target)
        os.tryrm(target:targetfile())
    end)

    -- xmake test runs these through on_test below, all three by default.
    add_tests("gofmt")
    add_tests("vet")
    add_tests("unit")

    on_test(function (target, opt)
        -- The name arrives qualified by the target: "nem/vet".
        local name = opt.name:match("[^/]+$")
        if name == "gofmt" then
            -- gofmt -l lists files that are not formatted and exits 0 either
            -- way, so the listing itself is the verdict.
            local out = os.iorunv("gofmt", {"-l", "."})
            if out:trim() ~= "" then
                cprint("${color.failure}not gofmt-formatted:\n%s", out)
                return false
            end
            return true
        end
        local args
        if name == "vet" then
            args = {"vet", "./..."}
        else
            args = {"test", "./...", "-race", "-count=1"}
        end
        local ok = try {
            function ()
                os.execv("go", args, {envs = {CGO_ENABLED = "1"}})
                return true
            end
        }
        return ok == true
    end)
target_end()

task("bench")
    set_category("plugin")
    on_run(function ()
        import("core.base.option")
        local args = {
            "test", "-run", "^$",
            "-bench", option.get("filter"),
            "-benchtime", option.get("time"),
            "-count", option.get("count"),
            "-benchmem",
        }
        local profile = option.get("cpuprofile")
        if profile then
            -- A profile belongs to one package's test binary, so this only
            -- makes sense with a single package.
            table.join2(args, {"-cpuprofile", path.absolute(profile)})
        end
        table.insert(args, option.get("packages"))
        os.execv("go", args, {envs = {CGO_ENABLED = "0"}})
    end)
    set_menu {
        usage = "xmake bench [options]",
        description = "Run nem's Go benchmarks.",
        options = {
            {'f', "filter",     "kv", ".",     "Run only the benchmarks matching this regexp."},
            {'t', "time",       "kv", "1s",    "How long to run each one, or how often: 1s, 500ms, 100x."},
            {'c', "count",      "kv", "1",     "Run each one this many times, for benchstat."},
            {'p', "packages",   "kv", "./...", "The packages to benchmark, e.g. ./editor."},
            {nil, "cpuprofile", "kv", nil,     "Write a CPU profile here (with a single package)."},
        }
    }
task_end()
