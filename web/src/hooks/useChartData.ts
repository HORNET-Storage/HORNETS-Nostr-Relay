import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

interface ChartDataItem {
  value: number;
  name: string;
}

const useChartData = () => {
    const [chartData, setChartData] = useState<ChartDataItem[] | null>(null);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const { t } = useTranslation();

    useEffect(() => {
        console.log("Component mounted, starting data fetch...");

        const fetchData = async () => {
            console.log("Preparing to fetch data...");
            setIsLoading(true);

            try {
                console.log("Sending request to server...");
                const response = await fetch('http://localhost:5000/relaycount', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({ relaycount: [] }) // No need to send kinds, server knows the categories
                });
                if (!response.ok) {
                    throw new Error(`Network response was not ok (status: ${response.status})`);
                }
                const data = await response.json(); // Expecting a response like { "kinds": 10, "photos": 5, "videos": 3, "gitNestr": 2 }
                console.log("Response Data:", data);

                // Process the data into chartDataItems using translated names
                const newChartData: ChartDataItem[] = [
                    { value: data.kinds, name: t('categories.kinds') },
                    { value: data.photos, name: t('categories.photos') },
                    { value: data.videos, name: t('categories.videos') },
                    { value: data.gitNestr, name: t('categories.gitNestr') }
                ];

                setChartData(newChartData);
            } catch (error) {
                console.error('Error:', error);
            } finally {
                console.log("Fetching process complete.");
                setIsLoading(false);
            }
        };

        fetchData();

        return () => {
            console.log("Cleanup called; Component unmounting...");
        };
    }, [t]);

    return { chartData, isLoading };
};

export default useChartData;


// import { useState, useEffect } from 'react';
// import { useTranslation } from 'react-i18next';  // Import useTranslation hook

// interface ChartDataItem {
//   value: number;
//   name: string;
// }

// const useChartData = () => {
//     const [chartData, setChartData] = useState<ChartDataItem[] | null>(null);
//     const [isLoading, setIsLoading] = useState<boolean>(true);
//     const { t } = useTranslation();  // Initialize the translation hook

//     useEffect(() => {
//         console.log("Component mounted, starting data fetch...");

//         const fetchData = async () => {
//             console.log("Preparing to fetch data...");
//             setIsLoading(true);
//             const kinds = [0, 1, 3, 5, 6, 10000, 1984, 30000, 30008, 30009, 30023, 36810, 7, 8, 9372, 9373, 9735, 9802]
//             ; // The kinds you want to count

//             try {
//                 console.log("Sending request to server...");
//                 const response = await fetch('http://localhost:5000/relaycount', {
//                     method: 'POST',
//                     headers: {
//                         'Content-Type': 'application/json'
//                     },
//                     body: JSON.stringify({ relaycount: kinds })
//                 });
//                 if (!response.ok) {
//                     throw new Error(`Network response was not ok (status: ${response.status})`);
//                 }
//                 const data = await response.json(); // Expecting a response like { "0": 2, "1": 3, ... }
//                 console.log("Response Data:", data);

//                 // Process the data into chartDataItems using translated names
//                 const newChartData = kinds.map(kind => ({
//                     value: data[kind.toString()],  // Access the response by converting kind to string key
//                     name: t(`checkboxes.kind${kind}`) // Use the translated name for each kind
//                 })).filter(item => item.value !== undefined); // Filter out any undefined values if kind was not in response

//                 setChartData(newChartData);
//             } catch (error) {
//                 console.error('Error:', error);
//             } finally {
//                 console.log("Fetching process complete.");
//                 setIsLoading(false);
//             }
//         };

//         fetchData();

//         return () => {
//             console.log("Cleanup called; Component unmounting...");
//         };
//     }, [t]);  // Add t to the dependency array to re-run the effect when the translation changes

//     return { chartData, isLoading };
// };

// export default useChartData;


