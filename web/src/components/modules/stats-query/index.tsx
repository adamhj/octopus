'use client';

import { useState, useCallback, useMemo, useEffect, Fragment } from 'react';
import dayjs from 'dayjs';
import {
    BarChart,
    Bar,
    XAxis,
    YAxis,
    CartesianGrid,
    Tooltip,
    ResponsiveContainer,
} from 'recharts';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';
import { PageWrapper } from '@/components/common/PageWrapper';
import { DateTimePicker } from '@/components/ui/datetime-picker';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { SearchableSelect } from '@/components/ui/searchable-select';
import { useStatsDetailChart } from '@/api/endpoints/stats';
import { useStatsQueryStore, type StatsPeriod } from './store';
import { useChannelOptions } from '@/api/endpoints/channel';
import { useModelOptions } from '@/api/endpoints/model';
import { logger } from '@/lib/logger';

// ==================== HSL 调色板 ====================

interface HslColor {
    h: number;
    s: number;
    l: number;
}

function hslColor(index: number, total: number, saturation: number, lightness: number): HslColor {
    const hue = Math.round((index / Math.max(total, 1)) * 360);
    return { h: hue, s: saturation, l: lightness };
}

function hslToString({ h, s, l }: HslColor): string {
    return `hsl(${h}, ${s}%, ${l}%)`;
}

function darkVariant({ h, s, l }: HslColor): string {
    return `hsl(${h}, ${s}%, ${Math.max(15, l - 25)}%)`;
}

function lightVariant({ h, s, l }: HslColor): string {
    // 浅色：降低饱和度 + 调高亮度
    return `hsl(${h}, ${Math.round(s * 0.6)}%, ${Math.min(90, l + 15)}%)`;
}

// ==================== 工具函数 ====================

function formatNumber(n: number): string {
    return n.toLocaleString('en-US');
}

function formatCost(n: number): string {
    return n.toFixed(4);
}

function comboKey(channelId: number, modelName: string): string {
    return `c${channelId}_${modelName.replace(/[^a-zA-Z0-9_-]/g, '_')}`;
}

function formatTimeLabel(time: string, period: StatsPeriod): string {
    switch (period) {
        case 'hour': {
            const m = time.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2})$/);
            if (m) return `${m[2]}-${m[3]} ${m[4]}:00`;
            return time;
        }
        case 'day': {
            const m = time.match(/^(\d{4})-(\d{2})-(\d{2})$/);
            if (m) return `${m[2]}-${m[3]}`;
            return time;
        }
        case 'week': {
            const start = dayjs(time);
            const end = start.add(6, 'day');
            return `${start.format('MM/DD')}-${end.format('MM/DD')}`;
        }
        case 'month': {
            const m = time.match(/^(\d{4})-(\d{2})$/);
            if (m) return `${m[1]}-${m[2]}`;
            return time;
        }
    }
}

/**
 * 根据数据点数量计算 X 轴间隔，自动稀疏标签避免重叠。
 * 数据点少时显示所有标签，数据点多时每隔若干个标签显示一个。
 */
function computeXAxisInterval(totalBuckets: number): number {
    if (totalBuckets <= 12) return 0; // 显示全部标签
    if (totalBuckets <= 48) return Math.ceil(totalBuckets / 8);
    return Math.ceil(totalBuckets / 6);
}

// ==================== 组合信息类型 ====================

interface ComboInfo {
    key: string;
    channelName: string;
    modelName: string;
}

// ==================== 自定义 Tooltip ====================

function CustomTooltip({ active, payload, combos, t }: {
    active?: boolean;
    payload?: Array<{ payload: Record<string, number> }>;
    combos: ComboInfo[];
    t: (key: string) => string;
}) {
    if (!active || !payload?.length) return null;
    const data = payload[0].payload;
    if (!data) return null;

    return (
        <div className="border-border/50 bg-background grid gap-1.5 rounded-lg border px-3 py-2 text-xs shadow-xl max-w-xs">
            {combos.map((combo) => {
                const missTokens = (data[`${combo.key}-miss`] as number) || 0;
                const hitTokens = (data[`${combo.key}-hit`] as number) || 0;
                const totalTokens = missTokens + hitTokens;
                if (totalTokens === 0) return null;

                const cacheHitRate = (hitTokens / totalTokens) * 100;
                const inputTokens = (data[`${combo.key}-input`] as number) || 0;
                const outputTokens = (data[`${combo.key}-output`] as number) || 0;
                const callCount = (data[`${combo.key}-calls`] as number) || 0;
                const cost = (data[`${combo.key}-cost`] as number) || 0;

                return (
                    <div key={combo.key} className="border-b pb-1 last:border-0 last:pb-0">
                        <div className="font-medium text-foreground mb-0.5">
                            {combo.channelName} / {combo.modelName}
                        </div>
                        <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-muted-foreground">
                            <span>{t('tooltip.inputTokens')}：</span><span className="text-right font-mono">{formatNumber(inputTokens)}</span>
                            <span>{t('tooltip.outputTokens')}：</span><span className="text-right font-mono">{formatNumber(outputTokens)}</span>
                            <span>{t('tooltip.cacheReadTokens')}：</span><span className="text-right font-mono">{formatNumber(hitTokens)}</span>
                            <span>{t('tooltip.cacheHitRate')}：</span><span className="text-right font-mono">{cacheHitRate.toFixed(2)}%</span>
                            <span>{t('tooltip.callCount')}：</span><span className="text-right font-mono">{formatNumber(callCount)}</span>
                            <span>{t('tooltip.cost')}：</span><span className="text-right font-mono">${formatCost(cost)}</span>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}

// ==================== 图例 ====================

function CustomLegend({ combos }: { combos: Array<ComboInfo & { color: HslColor }> }) {
    if (combos.length === 0) return null;

    return (
        <div className="flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 pt-3 pb-2">
            {combos.map((combo) => (
                <div key={combo.key} className="flex items-center gap-1.5">
                    <div className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ backgroundColor: darkVariant(combo.color) }} />
                    <span className="text-xs text-muted-foreground">{combo.channelName} / {combo.modelName}</span>
                </div>
            ))}
        </div>
    );
}

// ==================== 周期选项 ====================

const PERIOD_OPTIONS: Array<{ value: StatsPeriod; labelKey: string }> = [
    { value: 'hour', labelKey: 'period.hour' },
    { value: 'day', labelKey: 'period.day' },
    { value: 'week', labelKey: 'period.week' },
    { value: 'month', labelKey: 'period.month' },
];

// ==================== 主页面 ====================

export function StatsQuery() {
    const t = useTranslations('statsQuery');

    const committedParams = useStatsQueryStore();
    const submit = useStatsQueryStore((s) => s.submit);

    const [formStartTime, setFormStartTime] = useState<Date>(committedParams.startTime);
    const [formEndTime, setFormEndTime] = useState<Date>(committedParams.endTime);
    const [formChannelId, setFormChannelId] = useState<string>(
        committedParams.channelId !== undefined ? String(committedParams.channelId) : ''
    );
    const [formModelName, setFormModelName] = useState<string>(committedParams.modelName || '');
    const [formPeriod, setFormPeriod] = useState<StatsPeriod>(committedParams.period);

    // 渠道下拉选项 —— 通过 endpoint hook，无轮询
    const { data: channels } = useChannelOptions();

    const channelOptions = useMemo(() => {
        if (!channels) return [];
        return channels.map((ch) => ({
            value: String(ch.id),
            label: `${ch.name} (ID: ${ch.id})`,
        }));
    }, [channels]);

    // 模型下拉选项 —— 通过 endpoint hook，无轮询
    const { data: models } = useModelOptions();

    const modelOptions = useMemo(() => {
        if (!models) return [];
        return models.map((m) => ({ value: m.name, label: m.name }));
    }, [models]);

    const startTimeStr = dayjs(committedParams.startTime).format('YYYY-MM-DD HH:00');
    const endTimeStr = dayjs(committedParams.endTime).format('YYYY-MM-DD HH:00');

    const { data: chartData, isLoading, isFetching, error } = useStatsDetailChart({
        startTime: startTimeStr,
        endTime: endTimeStr,
        period: committedParams.period,
        channelId: committedParams.channelId,
        model: committedParams.modelName,
        _version: committedParams.queryVersion,
    });

    const querying = isLoading || isFetching;

    const allCombos = useMemo(() => {
        if (!chartData?.buckets) return [];
        const comboMap = new Map<string, ComboInfo>();
        for (const bucket of chartData.buckets) {
            for (const slot of bucket.slots) {
                const key = comboKey(slot.channel_id, slot.actual_model_name);
                if (!comboMap.has(key)) {
                    comboMap.set(key, {
                        key,
                        channelName: slot.channel_name || t('channelFallback', { id: slot.channel_id }),
                        modelName: slot.actual_model_name,
                    });
                }
            }
        }
        return Array.from(comboMap.values());
    }, [chartData, t]);

    const comboColors = useMemo(() => {
        return allCombos.map((combo, i) => ({
            ...combo,
            color: hslColor(i, allCombos.length, 55, 45),
        }));
    }, [allCombos]);

    const rechartsData = useMemo(() => {
        if (!chartData?.buckets) return [];

        return chartData.buckets.map((bucket) => {
            const row: Record<string, number | string> = {
                time: formatTimeLabel(bucket.time, committedParams.period),
            };

            for (const combo of allCombos) {
                const slot = bucket.slots.find(
                    (s) => comboKey(s.channel_id, s.actual_model_name) === combo.key
                );
                if (slot) {
                    const totalTokens = slot.input_token + slot.output_token;
                    const cacheMiss = totalTokens - slot.cache_read_tokens;
                    row[`${combo.key}-miss`] = Math.max(0, cacheMiss);
                    row[`${combo.key}-hit`] = slot.cache_read_tokens;
                    row[`${combo.key}-input`] = slot.input_token;
                    row[`${combo.key}-output`] = slot.output_token;
                    row[`${combo.key}-calls`] = slot.api_call_count;
                    row[`${combo.key}-cost`] = slot.input_cost + slot.output_cost;
                } else {
                    row[`${combo.key}-miss`] = 0;
                    row[`${combo.key}-hit`] = 0;
                    row[`${combo.key}-input`] = 0;
                    row[`${combo.key}-output`] = 0;
                    row[`${combo.key}-calls`] = 0;
                    row[`${combo.key}-cost`] = 0;
                }
            }
            return row;
        });
    }, [chartData, allCombos, committedParams.period]);

    const xAxisInterval = useMemo(
        () => computeXAxisInterval(rechartsData.length),
        [rechartsData.length]
    );

    const handleQuery = useCallback(() => {
        if (formStartTime.getTime() >= formEndTime.getTime()) {
            toast.error(t('error.startAfterEnd'));
            return;
        }
        submit({
            startTime: formStartTime,
            endTime: formEndTime,
            channelId: formChannelId ? Number(formChannelId) : undefined,
            modelName: formModelName || undefined,
            period: formPeriod,
        });
    }, [formStartTime, formEndTime, formChannelId, formModelName, formPeriod, submit, t]);

    // 查询出错时记录日志
    useEffect(() => {
        if (error) {
            logger.warn('用量查询 API 请求失败:', error);
        }
    }, [error]);

    const showEmpty = !querying && (!chartData?.buckets || allCombos.length === 0);

    return (
        <PageWrapper className="h-full min-h-0 overflow-y-auto overscroll-contain space-y-6 pb-24 md:pb-4 rounded-t-3xl">
            {/* 查询表单 */}
            <div className="rounded-3xl bg-card border-card-border border p-4 custom-shadow">
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('startTime')}</Label>
                        <DateTimePicker value={formStartTime} onChange={setFormStartTime} className="w-full" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('endTime')}</Label>
                        <DateTimePicker value={formEndTime} onChange={setFormEndTime} className="w-full" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('period.label')}</Label>
                        <Select value={formPeriod} onValueChange={(v) => setFormPeriod(v as StatsPeriod)}>
                            <SelectTrigger className="w-full">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {PERIOD_OPTIONS.map((opt) => (
                                    <SelectItem key={opt.value} value={opt.value}>{t(opt.labelKey)}</SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('channel')}</Label>
                        <SearchableSelect
                            value={formChannelId}
                            onChange={setFormChannelId}
                            options={channelOptions}
                            placeholder={t('channelPlaceholder')}
                            noMatchText={t('searchNoMatch')}
                        />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">{t('model')}</Label>
                        <SearchableSelect
                            value={formModelName}
                            onChange={setFormModelName}
                            options={modelOptions}
                            placeholder={t('modelPlaceholder')}
                            noMatchText={t('searchNoMatch')}
                        />
                    </div>
                    <div className="flex items-end">
                        <Button onClick={handleQuery} disabled={querying} className="w-full">
                            {querying ? t('querying') : t('query')}
                        </Button>
                    </div>
                </div>
                {error && (
                    <div className="mt-3 rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                        {error.message || t('error.apiError')}
                    </div>
                )}
            </div>

            {/* 堆叠柱状图 */}
            <div className="rounded-3xl bg-card border-card-border border pt-4 pb-0 custom-shadow">
                <div className="px-4 pb-2">
                    <h3 className="font-semibold text-base">{t('chartTitle')}</h3>
                </div>

                {showEmpty ? (
                    <div className="flex items-center justify-center h-64 text-muted-foreground text-sm">
                        {t('noData')}
                    </div>
                ) : (
                    <div className="h-72 w-full px-2">
                        <ResponsiveContainer width="100%" height="100%">
                            <BarChart data={rechartsData} barCategoryGap="15%">
                                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                                <XAxis
                                    dataKey="time"
                                    tickLine={false}
                                    axisLine={false}
                                    tick={{ fontSize: 11 }}
                                    interval={xAxisInterval}
                                />
                                <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 11 }} tickFormatter={formatNumber} />
                                <Tooltip content={<CustomTooltip combos={comboColors} t={t} />} cursor={{ fill: 'hsl(var(--muted-foreground) / 0.1)' }} />
                                {comboColors.map((combo) => (
                                    <Fragment key={combo.key}>
                                        <Bar
                                            dataKey={`${combo.key}-miss`}
                                            stackId="a"
                                            fill={darkVariant(combo.color)}
                                            isAnimationActive={false}
                                            activeBar={{ fill: hslToString(combo.color) }}
                                        />
                                        <Bar
                                            dataKey={`${combo.key}-hit`}
                                            stackId="a"
                                            fill={lightVariant(combo.color)}
                                            isAnimationActive={false}
                                            activeBar={{ fill: hslToString(combo.color) }}
                                        />
                                    </Fragment>
                                ))}
                            </BarChart>
                        </ResponsiveContainer>
                    </div>
                )}

                <CustomLegend combos={comboColors} />
            </div>
        </PageWrapper>
    );
}
