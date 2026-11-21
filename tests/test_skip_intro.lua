-- Run from the standalone cli folder: lua tests/test_skip_intro.lua
local clock, width, height = 0, 1280, 720
local timers, observers, events, messages, sections, enabled = {}, {}, {}, {}, {}, {}
local properties = {
    ["osd-dimensions"] = {}, ["video-params"] = {w=1280,h=720},
    ["pause"] = false, ["seekable"] = true, ["duration"] = 1420.02,
    ["mouse-pos"] = {x=0,y=0,hover=false},
}
local overrides = {mal_id=16498, episode=30}
local script_options
local seeks, shaders, requests, logs, area = {}, {}, {}, {}, nil
local overlays = {}
local function new_overlay()
    local overlay = {updates=0, removed=false}
    function overlay:update() self.updates=self.updates+1; self.removed=false end
    function overlay:remove() self.removed=true end
    overlays[#overlays+1] = overlay
    return overlay
end
local function timer(timeout, callback, periodic)
    local t = {active=true, deadline=clock+timeout, timeout=timeout, callback=callback, periodic=periodic}
    function t:kill() self.active=false end
    function t:resume() if not self.active then self.active=true; self.deadline=clock+self.timeout end end
    timers[#timers+1] = t
    return t
end

-- Enough JSON for AniSkip's answers.
local function decode(text)
    local i = 1
    local value
    local function skip() i = text:find("%S", i) or #text+1 end
    local function string_value() local j = text:find('"', i+1, true); local s = text:sub(i+1, j-1); i = j+1; return s end
    local function list(close, add)
        i = i+1; skip()
        if text:sub(i, i) == close then i = i+1; return end
        repeat add(); skip(); local separator = text:sub(i, i); i = i+1 until separator == close
    end
    value = function()
        skip()
        local c = text:sub(i, i)
        if c == "{" then
            local t = {}
            list("}", function() skip(); local k = string_value(); skip(); i = i+1; t[k] = value() end)
            return t
        elseif c == "[" then
            local t = {}
            list("]", function() t[#t+1] = value() end)
            return t
        elseif c == '"' then return string_value()
        elseif text:sub(i, i+3) == "true" then i = i+4; return true
        elseif text:sub(i, i+4) == "false" then i = i+5; return false
        elseif text:sub(i, i+3) == "null" then i = i+4; return nil end
        local number = text:match("^-?[%d.eE+-]+", i)
        i = i+#number
        return tonumber(number)
    end
    return value()
end

local mp = {
    get_time = function() return clock end,
    get_osd_size = function() return width,height,1 end,
    get_property_native = function(name, default) if properties[name] == nil then return default end; return properties[name] end,
    get_property_bool = function(name, default) if properties[name] == nil then return default end; return properties[name] end,
    get_property_number = function(name, default) if properties[name] == nil then return default end; return properties[name] end,
    set_property_native = function(name, value) properties[name]=value end,
    create_osd_overlay = new_overlay,
    set_key_bindings = function(bindings, name) sections[name]=bindings end,
    enable_key_bindings = function(name) enabled[name]=true end,
    disable_key_bindings = function(name) enabled[name]=false end,
    set_mouse_area = function(x,y,x1,y1,name) area={x=x,y=y,x1=x1,y1=y1,name=name} end,
    add_periodic_timer = function(interval, callback) return timer(interval,callback,true) end,
    add_timeout = function(interval, callback) return timer(interval,callback,false) end,
    observe_property = function(name, _, callback) observers[name]=callback; callback(name,properties[name]) end,
    register_event = function(name, callback) events[name]=callback end,
    register_script_message = function(name, callback) messages[name]=callback end,
    command_native_async = function(command, callback) requests[#requests+1] = {args=command.args, respond=callback} end,
    commandv = function(command, ...)
        local args = {...}
        if command == "seek" then
            seeks[#seeks+1] = {target=args[1], flags=args[2]}
            return true
        end
        assert(command == "change-list" and args[1] == "glsl-shaders", "The overlay must not change unrelated mpv settings")
        if args[2] == "append" then shaders[#shaders+1] = args[3] end
        if args[2] == "remove" then
            for i, path in ipairs(shaders) do if path == args[3] then table.remove(shaders, i) end end
        end
        return true
    end,
}
package.preload["mp"] = function() return mp end
package.preload["mp.msg"] = function()
    local function log(text) logs[#logs+1] = text end
    return {info=log, warn=log, error=log, verbose=log, debug=log}
end
package.preload["mp.utils"] = function() return {parse_json=decode} end
package.preload["mp.options"] = function()
    return {read_options=function(opts) for k, v in pairs(overrides) do opts[k] = v end; script_options=opts end}
end
dofile(arg[1] or "player/skip_intro.lua")
local overlay = overlays[1]

local function advance(seconds)
    for _=1,math.ceil(seconds*60) do
        clock=clock+1/60
        for _, t in ipairs(timers) do
            if t.active and t.deadline <= clock+0.00001 then
                if t.periodic then t.deadline=clock+t.timeout else t.active=false end
                t.callback()
            end
        end
    end
end
local function property(name, value)
    properties[name]=value
    if observers[name] then observers[name](name,value) end
end
local function mouse(x,y,hover)
    property("mouse-pos",{x=x,y=y,hover=hover ~= false})
end
local function button() return properties["user-data/animewatcher/skip-button"] end
local section = "animewatcher-skip-button"
local function url(request) return request.args[#request.args] end
local function respond(body)
    local request = table.remove(requests, 1)
    assert(request, "Expected an AniSkip request")
    request.respond(true, {status=0, stdout=body})
    return request
end
local function click()
    local r = button()
    mouse(r.x+r.width/2, r.y+r.height/2)
    sections[section][1][3]() -- mouse down
    advance(0.1)
    sections[section][1][4]() -- mouse up
    advance(0.1)
end
local function next_video(duration)
    events["end-file"]()
    events["start-file"]()
    properties.duration = duration
    property("time-pos", 0)
    property("video-params",{w=width,h=height})
    events["file-loaded"]()
end
local function exists(path) local file = io.open(path) if file then file:close() end; return file ~= nil end

-- Episode 30 of Attack on Titan, numbered on from season 1: AniSkip's rules move it to
-- season 2 episode 5, which has no skip times for this video's length, but has some for
-- another release.
property("time-pos", 0)
events["file-loaded"]()
local rules = respond('{"found":true,"rules":[{"from":{"start":26,"end":37},"to":{"malId":25777,"start":1,"end":12}},'
    .. '{"from":{"start":38},"to":{"malId":35760,"start":1}}],"statusCode":200}')
assert(url(rules) == "https://api.aniskip.com/v2/relation-rules/16498", "Relation rules are asked for the catalogue's anime")
local arguments = table.concat(rules.args, " ")
assert(rules.args[1] == "curl" and arguments:find("--globoff", 1, true) and arguments:find("--user-agent", 1, true),
    "curl must not glob AniSkip's types[] and must name itself to its firewall")
local exact = respond('{"found":false,"results":[],"statusCode":404}')
assert(url(exact) == "https://api.aniskip.com/v2/skip-times/25777/5?types[]=op&types[]=mixed-op&episodeLength=1420.020",
    "Skip times are first asked for this video's length, in the redirected season")
local any = respond('{"found":true,"results":[{"interval":{"startTime":10,"endTime":100},"skipType":"mixed-op","episodeLength":1467},'
    .. '{"interval":{"startTime":18.7,"endTime":109.2},"skipType":"op","episodeLength":1467.08}],"statusCode":200}')
assert(url(any):sub(-16) == "&episodeLength=0", "Then for a video of any length")
assert(button().intro_start == 18.7 and button().intro_end == 109.2, "A plain opening beats one mixed into the episode")
assert(#requests == 0, "One lookup per video")

advance(0.8)
assert(not button().visible and not enabled[section], "No button before the intro starts")
property("time-pos", 20)
advance(0.5)
assert(button().visible and enabled[section], "The button must appear when the intro starts")
assert(overlay.data:find("Skip intro",1,true), "The label must be present")
assert(area.x >= 0 and area.y >= 0 and area.x1 <= width and area.y1 < height-60,
    "The input area must cover only the button, above the standard controls")
local initial_updates=overlay.updates
advance(0.5)
assert(overlay.updates == initial_updates, "A settled button must stop redrawing")

assert(#shaders == 1, "The glass shader is added while the button is up")
local shader = io.open(shaders[1]):read("a")
assert(shader:find("//!HOOK OUTPUT", 1, true), "The glass refracts mpv's final output")
local cx, cy = shader:match("aw_center = vec2%(([%d.]+), ([%d.]+)%)")
local r = button()
assert(math.abs(cx - (r.x+r.width/2)/width) < 1e-4 and math.abs(cy - (r.y+r.height/2)/height) < 1e-4,
    "The shader must sit exactly under the drawn button")
local first_shader = shaders[1]

advance(8)
assert(not button().visible and not enabled[section] and #shaders == 0,
    "Without the mouse moving, the button and its glass leave after the prompt")
mouse(40,40)
advance(0.5)
assert(button().visible and #shaders == 1 and shaders[1] == first_shader,
    "Moving the mouse during the intro brings it back with the same shader")

click()
assert(#seeks == 1 and seeks[1].target == "109.200" and seeks[1].flags == "absolute+exact",
    "One completed click must seek exactly to the end of the intro")
assert(not button().visible and not enabled[section] and overlay.removed and #shaders == 0,
    "A successful click must immediately dismiss the button, its input area and its glass")
assert(properties.pause == false, "Skipping must preserve the playback state")

mouse(40,60)
property("pause",true)
property("osd-dimensions",{})
property("video-params",{w=1280,h=720})
script_options.always_visible=true
messages.show()
messages.skip()
advance(5)
assert(not button().visible and #seeks == 1,
    "While still in the intro, nothing may rearm a dismissed button or skip twice")
script_options.always_visible=false
property("pause",false)
property("time-pos", 109.2)
property("time-pos", 30)
advance(0.5)
assert(button().visible, "Seeking back into the intro offers the skip again")
property("time-pos", 107.5)
advance(0.5)
assert(not button().visible and not enabled[section], "The button leaves when the intro is about to end")

property("time-pos", 30)
advance(0.5)
mouse(30,30)
sections[section][1][2]()
assert(#seeks == 1, "Clicks outside the button must not seek")
r=button()
mouse(r.x+r.width/2,r.y+r.height/2)
sections[section][1][3]()
mouse(30,30)
sections[section][1][4]()
assert(#seeks == 1, "Dragging outside the button must cancel the click")
r=button()
mouse(r.x+r.width/2,r.y+r.height/2)
property("seekable",false) -- not observed: live stream state is read on click
sections[section][1][2]()
assert(#seeks == 1, "Unseekable streams must not be skipped")
properties.seekable=true
mouse(0,0,false)
advance(9)
assert(not button().visible and not enabled[section] and overlay.removed and #shaders == 0,
    "An invisible button must leave no invisible mouse input area or glass")

property("pause",true)
advance(0.8)
assert(button().visible, "The button should remain discoverable while paused")
r=button()
mouse(r.x+r.width/2,r.y+r.height/2)
sections[section][1][2]()
assert(#seeks == 2 and properties.pause == true, "Paused seeking must remain paused")
assert(not button().visible and not enabled[section], "A paused video must also stay dismissed")
property("pause",false)

-- Resizing moves the button at once; the glass follows once the window holds still.
property("time-pos", 109.2)
property("time-pos", 40)
advance(0.5)
assert(#shaders == 1, "The button is up again")
local before = shaders[1]
width,height=1600,900
property("osd-dimensions",{})
advance(0.1)
assert(#shaders == 0, "No misplaced glass while the window is being resized")
advance(0.5)
assert(#shaders == 1 and shaders[1] ~= before, "The glass returns in the button's new place")
assert(button().x+button().width <= width and button().y+button().height <= height,
    "Resizing must move both the rendered button and its click area")

-- A video AniSkip doesn't know gets no button.
next_video(1440)
respond('{"found":false,"rules":[],"statusCode":404}')
assert(url(respond('{"found":false,"results":[],"statusCode":404}')):find("/skip-times/16498/30?", 1, true),
    "Without a rule the catalogue's own numbering is used")
respond('{"found":false,"results":[],"statusCode":404}')
property("time-pos", 20)
advance(1)
assert(not button().visible and #shaders == 0, "No intro, no button")
assert(logs[#logs]:find("no intro", 1, true), "The terminal says why there is no button")

-- An answer for a video that is no longer playing is ignored.
next_video(1440)
local stale = table.remove(requests, 1)
next_video(1440)
stale.respond(true, {status=0, stdout='{"found":true,"rules":[],"statusCode":200}'})
assert(#requests == 1, "A stale answer must not continue its lookup")
respond('{"found":false,"rules":[],"statusCode":404}')
respond('{"found":true,"results":[{"interval":{"startTime":5,"endTime":95},"skipType":"op","episodeLength":1440}],"statusCode":200}')
property("time-pos", 6)
advance(0.5)
assert(button().visible and button().intro_end == 95, "The current video's intro is used")

-- Intros longer than the video are another release's: no button.
next_video(60)
respond('{"found":false,"rules":[],"statusCode":404}')
respond('{"found":false,"results":[],"statusCode":404}')
respond('{"found":true,"results":[{"interval":{"startTime":0,"endTime":90},"skipType":"op","episodeLength":1440}],"statusCode":200}')
property("time-pos", 20)
advance(0.5)
assert(not button().visible, "An intro ending after the video is ignored")

-- A failed request (no network, no curl) is not an intro.
next_video(1440)
table.remove(requests, 1).respond(false, nil)
assert(#requests == 1, "Without the rules, the catalogue's numbering is still tried")
table.remove(requests, 1).respond(true, {status=6, stdout=""})
assert(#requests == 1, "A failed request falls back to any length")
table.remove(requests, 1).respond(true, {status=0, stdout="error code: 1010"})
property("time-pos", 20)
advance(0.5)
assert(not button().visible, "Errors show no button")

-- Intro times given on the command line need no lookup, and refraction can be turned off.
script_options.intro_start, script_options.intro_end, script_options.refraction = 2, 12, false
next_video(60)
assert(#requests == 0, "Script options replace AniSkip")
property("time-pos", 3)
advance(0.5)
assert(button().visible and #shaders == 0, "Without refraction the glass is drawn without a shader")
click()
assert(seeks[#seeks].target == "12.000", "The button skips to the given end")

script_options.refraction = true
property("time-pos", 12)
property("time-pos", 3)
advance(0.5)
assert(#shaders == 1, "Refraction returns when turned back on")
local files = {}
for _, path in ipairs({first_shader, before, shaders[1]}) do files[#files+1] = path end
events["end-file"]()
advance(0.8)
assert(not button().visible and not enabled[section] and #shaders == 0, "No button or glass may linger after video ends")
events.shutdown()
assert(properties["user-data/animewatcher/skip-button"] == nil, "Shutdown must clean up")
for _, path in ipairs(files) do assert(not exists(path), "Shutdown deletes the glass shaders") end
print("PASS: AniSkip lookup with relation rules and length fallback, intro-only button, exact skip to the intro's end, "
    .. "re-arming, drag cancellation, auto-hide, pause, resizing, glass shader lifecycle, and cleanup")
