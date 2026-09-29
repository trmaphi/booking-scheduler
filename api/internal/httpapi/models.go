package httpapi

type bookingOptionsResponse struct {
	Vehicles     []vehicleResponse     `json:"vehicles"`
	Dealerships  []dealershipResponse  `json:"dealerships"`
	ServiceTypes []serviceTypeResponse `json:"serviceTypes"`
}

type vehicleResponse struct {
	ID           string `json:"id"`
	CustomerID   string `json:"customerId"`
	Label        string `json:"label"`
	Registration string `json:"registration"`
}

type dealershipResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Timezone string `json:"timezone"`
}

type serviceTypeResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DurationMinutes int    `json:"durationMinutes"`
}

type availabilityResponse struct {
	Slots []availabilitySlotResponse `json:"slots"`
}

type availabilitySlotResponse struct {
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
}

type confirmAppointmentRequest struct {
	VehicleID     string `json:"vehicleId"`
	DealershipID  string `json:"dealershipId"`
	ServiceTypeID string `json:"serviceTypeId"`
	StartAt       string `json:"startAt"`
}

type appointmentResponse struct {
	ID          string                   `json:"id"`
	Status      string                   `json:"status"`
	Vehicle     vehicleResponse          `json:"vehicle"`
	Dealership  dealershipResponse       `json:"dealership"`
	ServiceType serviceTypeResponse      `json:"serviceType"`
	Technician  assignedResourceResponse `json:"technician"`
	ServiceBay  assignedResourceResponse `json:"serviceBay"`
	StartAt     string                   `json:"startAt"`
	EndAt       string                   `json:"endAt"`
}

type assignedResourceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
