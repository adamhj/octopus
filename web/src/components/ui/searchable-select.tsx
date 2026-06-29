'use client';

import React, { useState, useCallback, useMemo } from 'react';
import { X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { cn } from '@/lib/utils';

/** onBlur 后延迟关闭弹窗的时间，确保点击下拉选项的 onClick 先触发 */
const BLUR_CLOSE_DELAY_MS = 150;

export interface SearchableSelectOption {
    value: string;
    label: string;
}

export interface SearchableSelectProps {
    value: string;
    onChange: (value: string) => void;
    options: SearchableSelectOption[];
    placeholder: string;
    className?: string;
    /** 无匹配结果时显示的文本 */
    noMatchText: string;
}

/**
 * 可搜索下拉框 — 输入框即筛选框。
 * 用户输入时过滤下拉选项，选中后显示 label，右侧有清除按钮。
 */
export function SearchableSelect({ value, onChange, options, placeholder, className, noMatchText }: SearchableSelectProps) {
    const [open, setOpen] = useState(false);
    const [inputText, setInputText] = useState('');

    const selectedLabel = useMemo(
        () => options.find((o) => o.value === value)?.label || '',
        [options, value]
    );

    // 下拉框打开时显示用户输入；关闭时显示已选 label
    const displayText = open ? inputText : (value ? selectedLabel : '');

    const filtered = useMemo(() => {
        const s = inputText.toLowerCase();
        if (!s) return options;
        return options.filter(
            (o) => o.label.toLowerCase().includes(s) || o.value.toLowerCase().includes(s)
        );
    }, [options, inputText]);

    const handleFocus = useCallback(() => {
        setInputText('');
        setOpen(true);
    }, []);

    const handleSelect = useCallback(
        (optValue: string) => {
            onChange(optValue === value ? '' : optValue);
            setInputText('');
            setOpen(false);
        },
        [value, onChange]
    );

    const handleClear = useCallback(
        (e: React.MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();
            onChange('');
            setInputText('');
            setOpen(false);
        },
        [onChange]
    );

    return (
        <Popover open={open && options.length > 0} onOpenChange={setOpen} modal={false}>
            <div className="relative">
                <PopoverAnchor asChild>
                    <Input
                        role="combobox"
                        aria-expanded={open}
                        placeholder={placeholder}
                        value={displayText}
                        onChange={(e) => {
                            setInputText(e.target.value);
                            setOpen(true);
                            if (e.target.value === '' && value) {
                                onChange('');
                            }
                        }}
                        onFocus={handleFocus}
                        onBlur={() => {
                            setTimeout(() => {
                                setOpen(false);
                                setInputText('');
                            }, BLUR_CLOSE_DELAY_MS);
                        }}
                        className={cn('w-full pr-8', className)}
                    />
                </PopoverAnchor>
                {value && (
                    <button
                        type="button"
                        className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={handleClear}
                        tabIndex={-1}
                    >
                        <X className="h-4 w-4" />
                    </button>
                )}
            </div>
            <PopoverContent
                className="p-1"
                style={{
                    width: 'fit-content',
                    minWidth: 'var(--radix-popover-trigger-width)',
                    maxWidth: 'calc(100vw - 2rem)',
                } as React.CSSProperties}
                align="start"
                side="bottom"
                onOpenAutoFocus={(e) => e.preventDefault()}
            >
                <div className="flex flex-col max-h-48 overflow-y-auto">
                    {filtered.map((opt) => (
                        <button
                            key={opt.value}
                            type="button"
                            className={cn(
                                'rounded-sm px-2 py-1.5 text-sm text-left whitespace-nowrap hover:bg-accent hover:text-accent-foreground',
                                value === opt.value && 'bg-accent text-accent-foreground'
                            )}
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => handleSelect(opt.value)}
                        >
                            {opt.label}
                        </button>
                    ))}
                    {filtered.length === 0 && (
                        <div className="px-2 py-1.5 text-sm text-muted-foreground">{noMatchText}</div>
                    )}
                </div>
            </PopoverContent>
        </Popover>
    );
}
