import dayjs, { Dayjs } from 'dayjs';
import LocalizedFormat from 'dayjs/plugin/localizedFormat';
import localeData from 'dayjs/plugin/localeData';
import isBetween from 'dayjs/plugin/isBetween';
import 'dayjs/locale/de';

dayjs.extend(LocalizedFormat);
dayjs.extend(localeData);
dayjs.extend(isBetween);

export type AppDate = Dayjs;

export class Dates {
  static setLocale(locale: string): void {
    dayjs.locale(locale);
  }

  static getToday(): AppDate {
    return dayjs();
  }

  static getClearDate(): AppDate {
    return this.getToday().hour(0).minute(0).second(0).millisecond(0);
  }

  static getMonths(): string[] {
    return dayjs.months();
  }

  static getDays(): string[] {
    return dayjs.weekdaysShort();
  }

  static getDate(date: number | string): AppDate {
    return dayjs(date);
  }

  static format(date: AppDate | string | number, query: string): string {
    if (typeof date === 'string' || typeof date === 'number') {
      return dayjs(date).format(query);
    } else {
      return date.format(query);
    }
  }
}
