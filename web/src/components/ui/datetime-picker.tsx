"use client"

import * as React from "react"
import dayjs from "dayjs"
import customParseFormat from "dayjs/plugin/customParseFormat"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { cn } from "@/lib/utils"

dayjs.extend(customParseFormat)

export type DateTimePickerProps = {
  value: Date
  onChange: (date: Date) => void
  className?: string
}

const DATE_FORMAT = "YYYY-MM-DD"

/** 将日期与小时合并为本地时间的 Date（分秒毫秒归零） */
export function mergeDateAndHour(datePart: Date, hour: number): Date {
  const d = dayjs(datePart)
  return d.hour(hour).minute(0).second(0).millisecond(0).toDate()
}

function clampHour(h: number): number {
  if (!Number.isFinite(h)) return 0
  return Math.min(23, Math.max(0, Math.floor(h)))
}

function parseDateText(text: string): Date | null {
  const parsed = dayjs(text.trim(), DATE_FORMAT, true)
  if (!parsed.isValid()) return null
  return parsed.startOf("day").toDate()
}

function parseHourText(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === "") return null
  const n = Number.parseInt(trimmed, 10)
  if (!Number.isFinite(n) || n < 0 || n > 23) return null
  return n
}

function DateTimePicker({ value, onChange, className }: DateTimePickerProps) {
  const valueDay = dayjs(value)
  const [dateOpen, setDateOpen] = React.useState(false)
  const [hourOpen, setHourOpen] = React.useState(false)
  const [dateText, setDateText] = React.useState(() => valueDay.format(DATE_FORMAT))
  const [hourText, setHourText] = React.useState(() => String(valueDay.hour()))

  React.useEffect(() => {
    const d = dayjs(value)
    setDateText(d.format(DATE_FORMAT))
    setHourText(String(d.hour()))
  }, [value])

  const emitChange = React.useCallback(
    (nextDatePart: Date, nextHour: number) => {
      onChange(mergeDateAndHour(nextDatePart, clampHour(nextHour)))
    },
    [onChange]
  )

  const commitDateText = React.useCallback(() => {
    const parsed = parseDateText(dateText)
    if (parsed) {
      const hour = parseHourText(hourText) ?? dayjs(value).hour()
      setDateText(dayjs(parsed).format(DATE_FORMAT))
      emitChange(parsed, hour)
    } else {
      setDateText(dayjs(value).format(DATE_FORMAT))
    }
  }, [dateText, hourText, value, emitChange])

  const commitHourText = React.useCallback(() => {
    const hour = parseHourText(hourText)
    if (hour !== null) {
      const normalized = String(hour)
      setHourText(normalized)
      emitChange(value, hour)
    } else {
      setHourText(String(dayjs(value).hour()))
    }
  }, [hourText, value, emitChange])

  const calendarSelected = React.useMemo(() => {
    const parsed = dayjs(dateText, DATE_FORMAT, true)
    return parsed.isValid() ? parsed.toDate() : value
  }, [dateText, value])

  return (
    <div
      className={cn("flex items-center gap-2", className)}
      data-slot="datetime-picker"
    >
      <Popover open={dateOpen} onOpenChange={setDateOpen} modal={false}>
        <PopoverAnchor asChild>
          <Input
            aria-label="Date"
            className="w-[9.5rem] font-mono tabular-nums"
            value={dateText}
            onChange={(e) => setDateText(e.target.value)}
            onFocus={() => setDateOpen(true)}
            onBlur={() => {
              commitDateText()
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                commitDateText()
                setDateOpen(false)
              }
            }}
          />
        </PopoverAnchor>
        <PopoverContent
          className="w-auto p-0"
          align="start"
          side="bottom"
          onOpenAutoFocus={(e) => e.preventDefault()}
          onInteractOutside={(e) => {
            const target = e.target as HTMLElement
            if (target.closest('[data-slot="datetime-picker"]')) {
              e.preventDefault()
            }
          }}
        >
          <div onMouseDown={(e) => e.preventDefault()}>
          <Calendar
            mode="single"
            selected={calendarSelected}
            onSelect={(d) => {
              if (!d) return
              const hour = parseHourText(hourText) ?? dayjs(value).hour()
              setDateText(dayjs(d).format(DATE_FORMAT))
              emitChange(d, hour)
              setDateOpen(false)
            }}
            defaultMonth={calendarSelected}
          />
          </div>
        </PopoverContent>
      </Popover>

      <Popover open={hourOpen} onOpenChange={setHourOpen} modal={false}>
        <PopoverAnchor asChild>
          <Input
            aria-label="Hour"
            className="w-[3.25rem] font-mono tabular-nums text-center"
            inputMode="numeric"
            value={hourText}
            onChange={(e) => setHourText(e.target.value)}
            onFocus={() => setHourOpen(true)}
            onBlur={() => {
              commitHourText()
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                commitHourText()
                setHourOpen(false)
              }
            }}
          />
        </PopoverAnchor>
        <PopoverContent
          className="w-auto p-1"
          align="start"
          side="bottom"
          onOpenAutoFocus={(e) => e.preventDefault()}
          onInteractOutside={(e) => {
            const target = e.target as HTMLElement
            if (target.closest('[data-slot="datetime-picker"]')) {
              e.preventDefault()
            }
          }}
        >
          <ul
            className="max-h-48 overflow-y-auto grid grid-cols-4 gap-0.5"
            role="listbox"
            aria-label="Hour"
          >
            {Array.from({ length: 24 }, (_, h) => (
              <li key={h} role="presentation">
                <button
                  type="button"
                  role="option"
                  aria-selected={dayjs(value).hour() === h}
                  className={cn(
                    "w-full rounded-sm px-2 py-1 text-sm font-mono tabular-nums hover:bg-accent hover:text-accent-foreground",
                    dayjs(value).hour() === h &&
                      "bg-primary text-primary-foreground hover:bg-primary hover:text-primary-foreground"
                  )}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    setHourText(String(h))
                    emitChange(value, h)
                    setHourOpen(false)
                  }}
                >
                  {h.toString().padStart(2, "0")}
                </button>
              </li>
            ))}
          </ul>
        </PopoverContent>
      </Popover>
    </div>
  )
}

export { DateTimePicker }