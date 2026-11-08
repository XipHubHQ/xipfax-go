// Package xipfax sends faxes from your code with XipFax, https://xipfax.com: quote a fax, add your
// PDFs, send them to a number or a government office, and follow each fax until it's delivered. It
// is generated from the API's own document, so its methods and types are the API's.
//
// Make a client from your API key, which you make in the developer console,
// https://developer.xipfax.com/console/, and keep on your server:
//
//	client, err := xipfax.New(os.Getenv("XIPFAX_API_KEY"))
//	if err != nil {
//		log.Fatal(err)
//	}
//	quote, _, err := client.QuoteAPI.GetQuote(ctx).Pages(3).Execute()
//
// The client's services are the API reference's sections, https://developer.xipfax.com/docs/api/:
// QuoteAPI, DraftAPI, FaxesAPI, PlanAPI, OfficesAPI and WebhookEndpointsAPI. Each request carries
// your key and the version of the API this SDK is generated from, APIVersion. Where the API refuses
// a request, its error is a *GenericOpenAPIError whose Model is the API's refusal, an APIError.
package xipfax
