interface Result {
  id: number;
  status: number;
  date: number;
  isActive: boolean;
}

export const patientResultsData: Array<Result> = [
  {
    id: 1,
    status: 1,
    date: Date.now(),
    isActive: true,
  },
  {
    id: 2,
    status: 2,
    date: Date.now(),
    isActive: false,
  },
  {
    id: 3,
    status: 3,
    date: Date.now() + 5 * 24 * 3600 * 1000,
    isActive: false,
  },
];
