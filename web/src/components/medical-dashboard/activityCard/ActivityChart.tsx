import React from 'react';
import { useTranslation } from 'react-i18next';
import { BaseChart, getDefaultTooltipStyles } from '@app/components/common/charts/BaseChart';
import { dashboardPaddings } from '@app/components/medical-dashboard/DashboardCard/DashboardCard';
import { useResponsive } from '@app/hooks/useResponsive';
import { ChartSeriesData } from '@app/interfaces/interfaces';
import { useAppSelector } from '@app/hooks/reduxHooks';
import { themeObject } from '@app/styles/themes/themeVariables';
import { graphic } from 'echarts';
import useActivityData from '@app/hooks/useActivityData';

export const ActivityChart: React.FC = () => {
  const theme = useAppSelector((state) => state.theme.theme);

  const { t } = useTranslation();
  const { data, isLoading } = useActivityData();

  const { isTablet, isDesktop, isMobile } = useResponsive();
  const size = isDesktop ? 'xl' : isTablet ? 'md' : isMobile ? 'xs' : 'xs';

  if (isLoading) {
    return <div>Loading...</div>;
  }

  const months = data.map(item => item.month);
  const values = data.map(item => item.total_gb);

  const option = {
    color: new graphic.LinearGradient(0, 0, 0, 1, [
      {
        offset: 0,
        color: 'rgba(51, 156, 253, 0.7)',
      },
      {
        offset: 1,
        color: 'rgba(51, 156, 253, 0.15)',
      },
    ]),
    grid: {
      top: dashboardPaddings[size][0],
      right: dashboardPaddings[size][1],
      bottom: dashboardPaddings[size][1],
      left: dashboardPaddings[size][0],
      containLabel: true,
    },
    xAxis: {
      type: 'category',
      axisTick: {
        show: false,
      },
      axisLine: {
        show: false,
      },
      data: months,
      position: 'bottom',
      nameLocation: 'middle',
      axisLabel: {
        color: themeObject[theme].primary,
        fontWeight: 500,
        fontSize: 14,
      },
    },
    yAxis: {
      type: 'value',
      min: 0,
      axisLabel: {
        formatter: '{value} GB',
        color: themeObject[theme].textLight,
        fontWeight: 500,
        fontSize: 14,
      },
    },
    series: [
      {
        barMaxWidth: 26,
        data: values,
        type: 'bar',
        itemStyle: {
          borderRadius: 7,
        },
      },
    ],
    tooltip: {
      ...getDefaultTooltipStyles(themeObject[theme]),
      trigger: 'axis',
      formatter: (data: ChartSeriesData) => {
        const currentItem = data[0];
        return `${currentItem.value} GB ${currentItem.name}`;
      },
    },
  };

  return <BaseChart option={option} height="100%" />;
};


// import React from 'react';
// import { useTranslation } from 'react-i18next';
// import { BaseChart, getDefaultTooltipStyles } from '@app/components/common/charts/BaseChart';
// import { dashboardPaddings } from '@app/components/medical-dashboard/DashboardCard/DashboardCard';
// import { useResponsive } from '@app/hooks/useResponsive';
// import { ChartSeriesData } from '@app/interfaces/interfaces';
// import { useAppSelector } from '@app/hooks/reduxHooks';
// import { themeObject } from '@app/styles/themes/themeVariables';
// import { graphic } from 'echarts';
// import useActivityData from '@app/hooks/useActivityData';

// export const ActivityChart: React.FC = () => {
//   const theme = useAppSelector((state) => state.theme.theme);

//   const { t } = useTranslation();
//   const { data, isLoading } = useActivityData();

//   const { isTablet, isDesktop, isMobile } = useResponsive();
//   const size = isDesktop ? 'xl' : isTablet ? 'md' : isMobile ? 'xs' : 'xs';

//   if (isLoading) {
//     return <div>Loading...</div>;
//   }

//   const months = data.map(item => item.month);
//   const values = data.map(item => item.total_gb);

//   const option = {
//     color: new graphic.LinearGradient(0, 0, 0, 1, [
//       {
//         offset: 0,
//         color: 'rgba(51, 156, 253, 0.7)',
//       },
//       {
//         offset: 1,
//         color: 'rgba(51, 156, 253, 0.15)',
//       },
//     ]),
//     grid: {
//       top: dashboardPaddings[size][0],
//       right: dashboardPaddings[size][1],
//       bottom: dashboardPaddings[size][1],
//       left: dashboardPaddings[size][0],
//       containLabel: true,
//     },
//     xAxis: {
//       type: 'category',
//       axisTick: {
//         show: false,
//       },
//       axisLine: {
//         show: false,
//       },
//       data: months,
//       position: 'top',
//       axisLabel: {
//         color: themeObject[theme].primary,
//         fontWeight: 500,
//         fontSize: 14,
//       },
//     },
//     yAxis: {
//       type: 'value',
//       min: 0,
//       axisLabel: {
//         formatter: '{value} GB',
//         color: themeObject[theme].textLight,
//         fontWeight: 500,
//         fontSize: 14,
//       },
//     },
//     series: [
//       {
//         barMaxWidth: 26,
//         data: values,
//         type: 'bar',
//         itemStyle: {
//           borderRadius: 7,
//         },
//       },
//     ],
//     tooltip: {
//       ...getDefaultTooltipStyles(themeObject[theme]),
//       trigger: 'axis',
//       formatter: (data: ChartSeriesData) => {
//         const currentItem = data[0];
//         return `${currentItem.value} GB ${currentItem.name}`;
//       },
//     },
//   };

//   return <BaseChart option={option} height="100%" />;
// };

