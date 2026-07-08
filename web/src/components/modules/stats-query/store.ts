import { create } from 'zustand';
import dayjs from 'dayjs';

/** 统计周期 */
export type StatsPeriod = 'hour' | 'day' | 'week' | 'month';

/** 查询表单提交参数 */
export interface StatsQueryParams {
    startTime: Date;
    endTime: Date;
    channelId?: number;
    modelName?: string;
    period: StatsPeriod;
}

/** 查询表单与已提交状态 */
interface StatsQueryState extends StatsQueryParams {
    /** 每次提交递增，确保同一参数也能触发重新请求 */
    queryVersion: number;
    /** 一次性提交所有表单参数，触发查询 */
    submit: (params: StatsQueryParams) => void;
}

function getDefaultTimeRange() {
    const now = dayjs();
    const nextHour = now.add(1, 'hour').minute(0).second(0).millisecond(0);
    const start = nextHour.subtract(24, 'hour');
    return { start: start.toDate(), end: nextHour.toDate() };
}

const defaults = getDefaultTimeRange();

export const useStatsQueryStore = create<StatsQueryState>()((set) => ({
    startTime: defaults.start,
    endTime: defaults.end,
    channelId: undefined,
    modelName: undefined,
    period: 'hour',
    queryVersion: 0,

    submit: (params) => set((s) => ({ ...params, queryVersion: s.queryVersion + 1 })),
}));
